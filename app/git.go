package main

// The Git tab of a feature: the repositories its sessions work in, what is committed and pushed there, and the
// pull or merge request of the branch. Everything is read with plain git, with the user's own keys and credential
// helpers, so nothing here can use the wrong account. Nothing is ever written to a repository, and git is told
// not to take its optional locks, so a session that works in the repository is not disturbed.
//
// A feature without a repository (its sessions never worked inside one) has no Git tab.

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	gitFast  = 3 * time.Second  // a local question
	gitSlow  = 10 * time.Second // a question to the remote
	gitKeep  = 8 * time.Second  // how long the local state of a feature is kept
	gitRefs  = 2 * time.Minute  // how long the answer of the remote is kept
	gitShown = 15               // commits listed per repository
)

// git runs one read-only git command in dir; ok is false when git failed or took too long.
func git(dir string, limit time.Duration, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return strings.TrimRight(out.String(), "\n"), err == nil
}

var (
	topCache   = map[string]string{} // folder -> the repository it is in ("" when in none)
	topCacheMu sync.Mutex
)

// repoOf is the top folder of the repository dir is in, "" when it is in none.
func repoOf(dir string) string {
	if dir == "" || !isDir(dir) {
		return ""
	}
	topCacheMu.Lock()
	top, ok := topCache[dir]
	topCacheMu.Unlock()
	if ok {
		return top
	}
	top, _ = git(dir, gitFast, "rev-parse", "--show-toplevel")
	topCacheMu.Lock()
	topCache[dir] = top
	topCacheMu.Unlock()
	return top
}

// sessionFolders are the folders a session worked in: where it was started, where it is now, the ones added to
// it and the ones it was in lately.
func sessionFolders(sid string) []string {
	var out []string
	if s := findSession(sid); s != nil && s.Cwd != "" {
		out = append(out, s.Cwd)
	}
	if info := transcriptInfo(sid); info != nil {
		out = append(out, info.Root, info.Dir)
		out = append(out, info.AddDirs...)
		out = append(out, info.Seen...)
	}
	return out
}

// featureRepos lists the repositories of a feature: every one its sessions worked in. They are remembered in
// repos.json next to feature.json, so a repository stays on the tab after its session has moved on or closed.
func featureRepos(slug string) []string {
	path := filepath.Join(featuresDir(), slug, "repos.json")
	known := strList(path)
	set := map[string]bool{}
	for _, r := range known {
		if isDir(filepath.Join(r, ".git")) || isFile(filepath.Join(r, ".git")) {
			set[r] = true
		}
	}
	for sid, f := range bindings() {
		if f != slug {
			continue
		}
		for _, dir := range sessionFolders(sid) {
			if top := repoOf(dir); top != "" {
				set[top] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	if strings.Join(out, "\n") != strings.Join(known, "\n") && isDir(filepath.Dir(path)) {
		l := make([]any, len(out))
		for i, r := range out {
			l[i] = r
		}
		_ = atomicWriteJSON(path, l)
	}
	return out
}

var (
	scpURL  = regexp.MustCompile(`^(?:[\w.-]+@)?([\w.-]+):(.+?)(?:\.git)?/?$`) // git@host:group/repo.git
	userInf = regexp.MustCompile(`^[^@/]*@`)
)

// webOf turns the address of a remote into the address of the repository's web page and its host.
// Credentials in the address are dropped: they must never reach a page.
func webOf(remote string) (web, host string) {
	remote = strings.TrimSpace(remote)
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil || u.Host == "" {
			return "", ""
		}
		host = u.Hostname()
		p := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
		if p == "" {
			return "", host
		}
		return "https://" + host + "/" + p, host
	}
	if m := scpURL.FindStringSubmatch(remote); m != nil && !strings.HasPrefix(m[2], "/") {
		return "https://" + m[1] + "/" + userInf.ReplaceAllString(m[2], ""), m[1]
	}
	return "", ""
}

// forgeOf says what kind of site hosts the repository: "github", "gitlab" or "" when it is not known.
// GitLab publishes merge requests as refs/merge-requests/N/head, GitHub pull requests as refs/pull/N/head,
// so the answer of the remote settles it for a self-hosted server with any name.
func forgeOf(host string, refs map[string]string) string {
	for ref := range refs {
		if strings.HasPrefix(ref, "refs/merge-requests/") {
			return "gitlab"
		}
		if strings.HasPrefix(ref, "refs/pull/") {
			return "github"
		}
	}
	if hosts, _ := config()["gitHosts"].(map[string]any); hosts != nil {
		if kind, _ := hosts[host].(string); kind == "github" || kind == "gitlab" {
			return kind
		}
	}
	switch {
	case host == "github.com":
		return "github"
	case strings.Contains(host, "gitlab"):
		return "gitlab"
	}
	return ""
}

type remoteRefs struct {
	at   time.Time
	refs map[string]string // ref -> commit
	ok   bool
}

var (
	refCache   = map[string]remoteRefs{}
	refCacheMu sync.Mutex
)

// remoteState asks the remote for its branches and its pull / merge requests. Kept for two minutes.
func remoteState(repo string) (map[string]string, bool) {
	refCacheMu.Lock()
	c, hit := refCache[repo]
	refCacheMu.Unlock()
	if hit && time.Since(c.at) < gitRefs {
		return c.refs, c.ok
	}
	out, ok := git(repo, gitSlow, "ls-remote", "origin", "refs/heads/*", "refs/merge-requests/*/head", "refs/pull/*/head")
	refs := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			refs[f[1]] = f[0]
		}
	}
	refCacheMu.Lock()
	refCache[repo] = remoteRefs{time.Now(), refs, ok}
	refCacheMu.Unlock()
	return refs, ok
}

func counts(repo, left, right string) (behind, ahead int, ok bool) {
	out, ok := git(repo, gitFast, "rev-list", "--left-right", "--count", left+"..."+right)
	f := strings.Fields(out)
	if !ok || len(f) != 2 {
		return 0, 0, false
	}
	behind, _ = strconv.Atoi(f[0])
	ahead, _ = strconv.Atoi(f[1])
	return behind, ahead, true
}

// repoState reads one repository. With remote=true the remote is asked too (branches and requests).
func repoState(repo string, mine []string, remote bool) map[string]any {
	r := map[string]any{"path": tilde(repo), "name": filepath.Base(repo)}
	// a linked worktree is a second folder of the same repository: it is named after that repository
	if common, ok := git(repo, gitFast, "rev-parse", "--path-format=absolute", "--git-common-dir"); ok {
		if main := filepath.Dir(common); filepath.Base(common) == ".git" && main != repo {
			r["name"], r["worktree"] = filepath.Base(main), filepath.Base(repo)
		}
	}
	branch, _ := git(repo, gitFast, "branch", "--show-current")
	head, _ := git(repo, gitFast, "rev-parse", "HEAD")
	if branch == "" && head != "" {
		r["detached"] = runes(head, 8)
	}
	r["branch"] = branch
	origin, hasOrigin := git(repo, gitFast, "remote", "get-url", "origin")
	web, host := "", ""
	if hasOrigin {
		web, host = webOf(origin)
	}
	r["web"], r["host"], r["remote"] = web, host, hasOrigin
	// the main branch of the repository, as the remote names it
	base := ""
	if s, ok := git(repo, gitFast, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); ok {
		base = strings.TrimPrefix(s, "origin/")
	} else {
		for _, b := range []string{"main", "master", "develop"} {
			if _, ok := git(repo, gitFast, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+b); ok {
				base = b
				break
			}
		}
	}
	r["base"] = base
	// not committed
	if out, ok := git(repo, gitFast, "status", "--porcelain"); ok {
		n := 0
		if out != "" {
			n = len(strings.Split(out, "\n"))
		}
		r["dirty"] = n
	}
	// not pushed: against the branch it follows, or against nothing when it follows none
	unpushed := map[string]bool{}
	_, hasUp := git(repo, gitFast, "rev-parse", "--verify", "--quiet", "@{upstream}")
	switch {
	case hasUp:
		if behind, ahead, ok := counts(repo, "@{upstream}", "HEAD"); ok {
			r["ahead"], r["behind"] = ahead, behind
		}
		if out, ok := git(repo, gitFast, "rev-list", "--max-count=200", "@{upstream}..HEAD"); ok && out != "" {
			for _, h := range strings.Split(out, "\n") {
				unpushed[h] = true
			}
		}
	case hasOrigin && branch != "":
		r["noUpstream"] = true // the branch was never pushed
		if out, ok := git(repo, gitFast, "rev-list", "--max-count=200", "HEAD", "--not", "--remotes=origin"); ok && out != "" {
			for _, h := range strings.Split(out, "\n") {
				unpushed[h] = true
			}
		}
		r["ahead"] = len(unpushed)
	}
	// how far the branch is from the main one
	onBase := branch != "" && branch == base
	if base != "" && !onBase {
		if behind, ahead, ok := counts(repo, "origin/"+base, "HEAD"); ok {
			r["baseAhead"], r["baseBehind"] = ahead, behind
		}
	}
	// the commits: those of the branch that the main one does not have; on the main branch, the latest ones
	span := []string{"-" + strconv.Itoa(gitShown), "HEAD"}
	if base != "" && !onBase {
		span = []string{"-" + strconv.Itoa(gitShown), "origin/" + base + "..HEAD"}
	}
	commits := []any{}
	if out, ok := git(repo, gitFast, append([]string{"log", "--format=%H%x1f%ct%x1f%an%x1f%s"}, span...)...); ok && out != "" {
		for _, line := range strings.Split(out, "\n") {
			f := strings.Split(line, "\x1f")
			if len(f) != 4 {
				continue
			}
			ts, _ := strconv.ParseInt(f[1], 10, 64)
			c := map[string]any{"hash": f[0], "short": runes(f[0], 7), "ts": ts * 1000, "author": f[2], "subject": f[3]}
			if hasOrigin { // without a remote there is nowhere to push: the question does not arise
				c["pushed"] = !unpushed[f[0]]
			}
			// made by a session of this feature: the hook wrote down the message it was committed with (cut at
			// 90 characters, the body joined to the subject)
			subj := runes(strings.Join(strings.Fields(f[3]), " "), 90)
			for _, m := range mine {
				if subj != "" && (strings.HasPrefix(m, subj) || strings.HasPrefix(subj, m)) {
					c["mine"] = true
					break
				}
			}
			commits = append(commits, c)
		}
	}
	r["commits"] = commits
	if !remote || !hasOrigin {
		return r
	}
	// the remote: is the branch there, and is there a pull / merge request of it
	refs, ok := remoteState(repo)
	r["asked"] = ok
	forge := forgeOf(host, refs)
	r["forge"] = forge
	if !ok {
		return r
	}
	tip := refs["refs/heads/"+branch]
	r["onRemote"] = tip != ""
	prefix := map[string]string{"gitlab": "refs/merge-requests/", "github": "refs/pull/"}[forge]
	if prefix == "" || branch == "" || onBase {
		return r
	}
	best := 0
	for ref, sha := range refs {
		if !strings.HasPrefix(ref, prefix) || (sha != tip && sha != head) || sha == "" {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(ref, prefix), "/head")); err == nil && n > best {
			best = n
		}
	}
	if best > 0 {
		req := map[string]any{"n": best}
		// merged, when the main branch of the remote already has the commit the request ends with
		// (a merged branch is often deleted on the remote: then the local end of it is what the request holds)
		end := tip
		if end == "" {
			end = head
		}
		if baseTip := refs["refs/heads/"+base]; baseTip != "" && end != "" {
			if _, ok := git(repo, gitFast, "merge-base", "--is-ancestor", end, baseTip); ok {
				req["merged"] = true
			}
		}
		// (the main branch of the remote may be newer than what was fetched: what was fetched answers as well)
		if req["merged"] == nil && end != "" && base != "" {
			if _, ok := git(repo, gitFast, "merge-base", "--is-ancestor", end, "origin/"+base); ok {
				req["merged"] = true
			}
		}
		r["request"] = req
	}
	return r
}

type gitCacheItem struct {
	at time.Time
	v  map[string]any
}

var (
	gitCache   = map[string]gitCacheItem{}
	gitCacheMu sync.Mutex
)

// gitState is the Git tab of a feature: {"repos": [...]}; no repositories, no tab. remote=false leaves the remote
// alone (fast, used when the page is built), remote=true also asks it for the branch and its request.
func gitState(slug string, remote bool) map[string]any {
	key := slug + map[bool]string{true: "+", false: "-"}[remote]
	gitCacheMu.Lock()
	c, hit := gitCache[key]
	gitCacheMu.Unlock()
	if hit && time.Since(c.at) < gitKeep {
		return c.v
	}
	// the commits the sessions of this feature made, as the hook saw them: `commit "subject"`
	var mine []string
	sids := map[string]bool{}
	for sid, f := range bindings() {
		if f == slug {
			sids[sid] = true
		}
	}
	if b, err := os.ReadFile(filepath.Join(home(), "events.jsonl")); err == nil {
		for _, line := range bytes.Split(b, []byte("\n")) {
			if !bytes.Contains(line, []byte(`"commit"`)) {
				continue
			}
			e, _ := parseJSON(line).(map[string]any)
			if e == nil || e["kind"] != "commit" || !sids[str(e, "session")] {
				continue
			}
			t := str(e, "text")
			if i := strings.IndexByte(t, '"'); i >= 0 && strings.HasSuffix(t, `"`) && len(t) > i+1 {
				mine = append(mine, t[i+1:len(t)-1])
			}
		}
	}
	// the other sessions that are in the same repository right now
	others := map[string][]any{}
	for _, s := range readSessions(nowMs(), false, true, false) {
		if sids[s.ID] || s.State == "closed" {
			continue
		}
		dir := s.Cwd
		if info := transcriptInfo(s.ID); info != nil && info.Dir != "" {
			dir = info.Dir
		}
		if top := repoOf(dir); top != "" {
			name := s.Name
			if name == "" {
				name = runes(s.ID, 8)
			}
			others[top] = append(others[top], map[string]any{"name": name, "feature": s.Feature, "state": s.State})
		}
	}
	repos := []any{}
	for _, repo := range featureRepos(slug) {
		r := repoState(repo, mine, remote)
		if o := others[repo]; len(o) > 0 {
			r["others"] = o
		}
		repos = append(repos, r)
	}
	v := map[string]any{"repos": repos, "remote": remote}
	gitCacheMu.Lock()
	gitCache[key] = gitCacheItem{time.Now(), v}
	gitCacheMu.Unlock()
	return v
}
