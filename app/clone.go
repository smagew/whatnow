package main

// New work from a repository named by its address: the dashboard clones it into a folder the user chose and
// starts the session there. The folder for clones is the user's choice every time (the page remembers the last
// one); the dashboard has no place of its own for other people's code.
//
// A repository that is already cloned in that folder is not cloned again: the session starts in what is there.
//
// The address goes to git, so it is checked first: only the forms that mean "a repository on a server" (https,
// http, ssh, git@host:path) are taken. Anything else git would accept is a way to run a program or to read a
// local path ("ext::sh -c …", "--upload-pack=…", "file://…"), and is refused.

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	cloneLimit = 10 * time.Minute
	// what git may talk over: no local paths, no "ext::" (which runs a command)
	cloneProtocols = "http:https:ssh"
	scpAddr        = regexp.MustCompile(`^([A-Za-z0-9._-]+@)?([A-Za-z0-9.-]+):([A-Za-z0-9._~/-]+)$`)
	cloneNameRe    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// repoAddr reads the address of a repository: key is the same for the https and the ssh address of one
// repository ("github.com/owner/name"), name is the folder a clone of it gets. ok is false for anything that is
// not an http(s) or ssh address.
func repoAddr(s string) (key, name string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "-") || len(s) > 500 || strings.ContainsAny(s, " \t\r\n\\'\"`$") {
		return "", "", false
	}
	host, path := "", ""
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh") {
			return "", "", false
		}
		host, path = u.Hostname(), u.Path
	} else if m := scpAddr.FindStringSubmatch(s); m != nil { // git@host:owner/name.git
		host, path = m[2], m[3]
	} else {
		return "", "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	name = path[strings.LastIndex(path, "/")+1:]
	if path == "" || !cloneNameRe.MatchString(name) || strings.Trim(name, ".") == "" {
		return "", "", false
	}
	return strings.ToLower(host) + "/" + path, name, true
}

// clonePlan says where a repository would be cloned, before anything is done: shown in the form and used to
// start. "exists" means the folder is there already and is a clone of this very repository.
func clonePlan(addr, into, mode string) (int, map[string]any) {
	key, name, ok := repoAddr(addr)
	if !ok {
		return 400, map[string]any{"error": "this is not the address of a repository: it looks like https://host/owner/name or git@host:owner/name.git"}
	}
	into = expandUser(strings.TrimSpace(into))
	if into == "" {
		return 400, map[string]any{"error": "choose the folder to clone into"}
	}
	if !filepath.IsAbs(into) {
		return 400, map[string]any{"error": "give the full path of the folder to clone into, from / or ~"}
	}
	into = filepath.Clean(into)
	if !isDir(into) {
		return 404, map[string]any{"error": "there is no such folder: " + tilde(into)}
	}
	if mode != "" && !modeRe.MatchString(mode) {
		return 400, map[string]any{"error": "bad permission mode"}
	}
	root := filepath.Join(into, name)
	exists := false
	if l, err := os.ReadDir(root); err == nil && len(l) > 0 { // an empty folder is as good as none
		top, _ := git(root, gitFast, "rev-parse", "--show-toplevel")
		origin, _ := git(root, gitFast, "config", "--get", "remote.origin.url")
		there, _, _ := repoAddr(origin)
		a, _ := filepath.EvalSymlinks(top)
		b, _ := filepath.EvalSymlinks(root)
		if top == "" || a != b || there != key {
			return 409, map[string]any{"error": tilde(root) + " is there already and is not a clone of this repository. Choose another folder to clone into."}
		}
		exists = true
	} else if err != nil && !os.IsNotExist(err) {
		return 409, map[string]any{"error": tilde(root) + " is there already and is not a folder"}
	}
	return 200, map[string]any{"repo": strings.TrimSpace(addr), "name": name, "root": root, "rootShown": tilde(root), "exists": exists,
		"git": exists, "dirs": []any{}, "dirsShown": []any{}, "mode": mode}
}

// cloneInto clones addr into root. Nothing is asked: git gets no terminal, so a repository that needs a password
// typed fails with a message instead of waiting. A clone that failed leaves nothing behind.
func cloneInto(addr, root string) (string, bool) {
	_, statErr := os.Stat(root)
	ctx, cancel := context.WithTimeout(context.Background(), cloneLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--", addr, root) // "--": nothing in the address is an option
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL="+cloneProtocols, "LC_ALL=C")
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err == nil {
		return "", true
	}
	if os.IsNotExist(statErr) {
		_ = os.RemoveAll(root)
	}
	if ctx.Err() != nil {
		return "the clone took longer than " + cloneLimit.String() + " and was stopped", false
	}
	var keep []string // what git said went wrong, without the progress
	for _, l := range strings.Split(strings.ReplaceAll(string(out), "\r", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "Cloning into") || strings.HasPrefix(l, "Receiving ") || strings.HasPrefix(l, "Resolving ") {
			continue
		}
		keep = append(keep, l)
	}
	if len(keep) > 4 {
		keep = keep[len(keep)-4:]
	}
	msg := strings.Join(keep, " ")
	if msg == "" {
		msg = err.Error()
	}
	low := strings.ToLower(msg)
	if strings.Contains(low, "terminal prompts disabled") || strings.Contains(low, "permission denied") || strings.Contains(low, "authentication failed") ||
		strings.Contains(low, "host key verification failed") {
		msg += " — git could not sign in, and the dashboard does not ask for passwords. Clone the repository once in a terminal, then start the work in that folder."
	}
	return runes(msg, 700), false
}

// cloneWork is "New work → From a repository": clone (unless it is there already), then a session in the clone.
func cloneWork(addr, into, mode, task, place string) (int, any) {
	code, p := clonePlan(addr, into, mode)
	if code != 200 {
		return code, p
	}
	root := str(p, "root")
	if p["exists"] != true {
		if msg, ok := cloneInto(str(p, "repo"), root); !ok {
			return 502, map[string]any{"error": "git could not clone the repository: " + msg}
		}
		place = "same" // nobody else works in a clone made a second ago
	}
	code, fp := folderPlan(root, mode)
	if code != 200 {
		return code, fp
	}
	code, res := startWork(fp, "", task, place)
	if m, ok := res.(map[string]any); ok && code != 200 {
		m["cloned"] = tilde(root) // the clone stays: Start again goes on from it
	}
	return code, res
}
