package main

// Reading and checking feature.json. Statuses are derived here, never typed
// by Claude. A file of the wrong shape gives a DataError with a message, never a crash.

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	factKinds = []string{"y", "fixed", "bad", "p", "q", "w"} // done, fixed problem, open problem, partly, not done, in work
	doneKinds = map[string]bool{"y": true, "fixed": true, "n": true}
	sevs      = []string{"high", "mid", "low"}
	whos      = []string{"you", "look", "agent", "later"}
)

// kinds says what a stage is about. A feature has only the ones its plan has: marks, not required steps.
var kinds = map[string][]string{"plan": {"📝", "Planning"}, "research": {"🔍", "Research"}, "code": {"🛠", "Writing code"},
	"test": {"🧪", "Testing"}, "review": {"👀", "Review"}, "deploy": {"🚀", "Deployment"}, "docs": {"📄", "Documentation"}}

var kindHead = regexp.MustCompile(`^(?:фаза|этап|phase|stage|step)\s*\d+[^.:]*[.:]\s*`)

var kindGuess = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"deploy", regexp.MustCompile(`^(?:deploy|release|rollout|выкат|выклад|релиз|деплой|подготовка к выкладке)`)},
	{"review", regexp.MustCompile(`^(?:(?:code[ -]?)?review|(?:код[ -]?)?ревью|аудит|audit)`)},
	{"test", regexp.MustCompile(`^(?:tests?\b|testing|тест|проверк|автопроверк|ручная проверка|сетка безопасности)`)},
	{"docs", regexp.MustCompile(`^(?:docs?\b|documentation|документ)`)},
	{"research", regexp.MustCompile(`^(?:research|analysis|investigat|анализ|исследован|разведка|разобраться|подготовка и инвентар)`)},
	{"plan", regexp.MustCompile(`^(?:plan|план|согласовать|спроектировать|проектирование)`)},
}

type DataError struct{ msg string }

func (e DataError) Error() string { return e.msg }

func fail(format string, a ...any) { panic(DataError{fmt.Sprintf(format, a...)}) }

func in(list []string, v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func pyTuple(l []string) string { return "('" + strings.Join(l, "', '") + "')" }

func kindOf(v any) string { s, _ := v.(string); return s }

// stageState says how worried to be, not whether anything is left: no = nothing done, bad = an open
// problem, ok = all done, almost = 80% or more done, part = the rest.
func stageState(facts []any) string {
	ks := make([]string, len(facts))
	allQ, bad, done := true, false, 0
	for i, f := range facts {
		k := kindOf(f.([]any)[0])
		if doneKinds[k] {
			k = "y"
		}
		ks[i] = k
		if k != "q" {
			allQ = false
		}
		if k == "bad" {
			bad = true
		}
		if k == "y" {
			done++
		}
	}
	switch {
	case allQ:
		return "no"
	case bad:
		return "bad"
	case done == len(ks):
		return "ok"
	case float64(done)/float64(len(ks)) >= 0.8:
		return "almost"
	}
	return "part"
}

func stageKind(s map[string]any) string {
	if k, ok := s["kind"]; ok && k != nil {
		if _, known := kinds[kindOf(k)]; !known {
			names := make([]string, 0, len(kinds))
			for _, n := range []string{"plan", "research", "code", "test", "review", "deploy", "docs"} {
				names = append(names, n)
			}
			fail("stage %v: kind must be one of %s", s["n"], strings.Join(names, ", "))
		}
		return k.(string)
	}
	// only what the title starts with counts: a word somewhere inside it ("..., tests") says too little
	title := kindHead.ReplaceAllString(strings.ToLower(strings.TrimSpace(fmt.Sprint(s["title"]))), "")
	for _, g := range kindGuess {
		if g.re.MatchString(title) {
			return g.kind
		}
	}
	return ""
}

// progress is the percent of the plan that is done. Every stage weighs the same and counts by the share of
// its items done (partly done = half, "not needed" left out); a stage without items counts as 0.
func progress(stages []any) int {
	if len(stages) == 0 {
		return 0
	}
	total := 0.0
	for _, x := range stages {
		facts := x.(map[string]any)["facts"].([]any)
		n, got := 0, 0.0
		for _, f := range facts {
			k := kindOf(f.([]any)[0])
			if k == "n" {
				continue
			}
			n++
			if doneKinds[k] {
				got++
			} else if k == "p" {
				got += .5
			}
		}
		if n > 0 {
			total += got / float64(n)
		} else if len(facts) > 0 {
			total++ // everything in it was marked as not needed
		}
	}
	return int(math.RoundToEven(100 * total / float64(len(stages))))
}

// currentStage is the last stage with an item in work, else the furthest stage that was started, else the first.
func currentStage(stages []any) any {
	for _, want := range []func(map[string]any) bool{
		func(s map[string]any) bool {
			for _, f := range s["facts"].([]any) {
				if kindOf(f.([]any)[0]) == "w" {
					return true
				}
			}
			return false
		},
		func(s map[string]any) bool { return s["state"] != "no" },
	} {
		var hit any
		for _, x := range stages {
			if s := x.(map[string]any); want(s) {
				hit = s["n"]
			}
		}
		if hit != nil {
			return hit
		}
	}
	if len(stages) > 0 {
		return stages[0].(map[string]any)["n"]
	}
	return nil
}

func listOf(d map[string]any, k string) []any {
	if v, ok := d[k]; ok {
		return v.([]any)
	}
	l := []any{}
	d[k] = l
	return l
}

// prepare validates a parsed feature.json and adds what the pages need. It changes d in place.
func prepare(d map[string]any, overrides map[string]any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if de, ok := r.(DataError); ok {
				err = de
			} else {
				// a value of the wrong shape: a list where an object is expected, an empty cell, ...
				err = DataError{fmt.Sprintf("wrong shape, see SCHEMA.md (%v)", r)}
			}
		}
	}()
	for _, k := range []string{"title", "projects", "stages"} {
		if _, ok := d[k]; !ok {
			fail(`missing "%s"`, k)
		}
	}
	projects, ok := d["projects"].([]any)
	if !ok || len(projects) == 0 {
		fail(`"projects" must be a non-empty list of names`)
	}
	known := map[string]bool{}
	for _, p := range projects {
		s, ok := p.(string)
		if !ok {
			fail(`"projects" must be a non-empty list of names`)
		}
		known[s] = true
	}
	for _, k := range []string{"sub", "summary"} {
		if _, ok := d[k]; !ok {
			d[k] = ""
		}
	}
	for _, k := range []string{"decisions", "questions", "blockers", "nextSteps"} {
		listOf(d, k)
	}
	stages := d["stages"].([]any)
	var last int64
	nums := map[int64]bool{}
	for _, x := range stages {
		s := x.(map[string]any)
		n, ok := intOf(s["n"])
		if !ok || n <= last {
			fail("stage numbers must be integers in increasing order (got %v)", s["n"])
		}
		last = n
		nums[n] = true
		for _, k := range []string{"title", "verdict", "plan", "facts"} {
			if _, ok := s[k]; !ok {
				fail(`stage %d: missing "%s"`, n, k)
			}
		}
		facts := s["facts"].([]any)
		for _, f := range facts {
			l, ok := f.([]any)
			if !ok || len(l) != 2 || !in(factKinds, l[0]) {
				fail("stage %d: each fact is [kind, text], kind in %s", n, pyTuple(factKinds))
			}
		}
		// ids of facts, and the user's "not needed" decisions (overrides.json, never written by Claude)
		fids, reasons := make([]any, len(facts)), map[string]any{}
		for i, f := range facts {
			fid := factID(n, f.([]any)[1])
			fids[i] = fid
			if o, ok := overrides[fid].(map[string]any); ok && o["kind"] == "skip" {
				if k := kindOf(f.([]any)[0]); k != "y" && k != "fixed" {
					facts[i] = []any{"n", f.([]any)[1]}
					r, ok := o["reason"]
					if !ok {
						r = ""
					}
					reasons[fmt.Sprint(i)] = r
				}
			}
		}
		s["fids"], s["reasons"] = fids, reasons
		s["state"] = stageState(facts)
		s["kind"] = stageKind(s)
		ks, dn := make([]any, len(facts)), 0
		for i, f := range facts {
			k := kindOf(f.([]any)[0])
			switch {
			case k == "fixed":
				ks[i] = "fixed"
			case doneKinds[k]:
				ks[i] = "y"
			default:
				ks[i] = k
			}
			if doneKinds[k] {
				dn++
			}
		}
		s["kinds"], s["dn"], s["tot"] = ks, dn, len(facts)
	}
	d["cur"] = currentStage(stages)

	// blockers must point at real stages and have a reason and an action
	for _, x := range d["blockers"].([]any) {
		b := x.(map[string]any)
		for _, k := range []string{"t", "why", "todo", "proj", "stage", "sev"} {
			if _, ok := b[k]; !ok {
				fail(`blocker "%v": missing "%s"`, b["t"], k)
			}
		}
		if !in(sevs, b["sev"]) {
			fail(`blocker "%v": sev must be one of %s`, b["t"], pyTuple(sevs))
		}
		if w, ok := b["who"]; ok && w != nil && !in(whos, w) {
			fail(`blocker "%v": who must be one of %s`, b["t"], pyTuple(whos))
		}
		b["bid"] = factID("b", b["t"])
		if n, ok := intOf(b["stage"]); !ok || !nums[n] {
			fail(`blocker "%v": stage %v does not exist`, b["t"], b["stage"])
		}
		for _, p := range b["proj"].([]any) {
			if s, ok := p.(string); !ok || !known[s] {
				fail(`blocker "%v": unknown project "%v"`, b["t"], p)
			}
		}
	}
	// What the user closed on the page ("done" or "not needed", overrides.json) is no longer open: it leaves
	// the list at once and is shown apart, until the session takes it out of feature.json.
	open, closed := []any{}, []any{}
	for _, x := range d["blockers"].([]any) {
		b := x.(map[string]any)
		o, _ := overrides[fmt.Sprint(b["bid"])].(map[string]any)
		if k := o["kind"]; k == "done" || k == "skip" {
			b["closed"], b["reason"], b["closedTs"] = k, o["reason"], o["ts"]
			closed = append(closed, b)
			continue
		}
		open = append(open, b)
	}
	d["blockers"], d["closedBlockers"] = open, closed

	// ready only when nothing blocks and every stage of the plan is done
	ready := len(d["blockers"].([]any)) == 0 && len(stages) > 0
	for _, x := range stages {
		if x.(map[string]any)["state"] != "ok" {
			ready = false
		}
	}
	d["ready"] = ready
	d["pct"] = progress(stages)
	d["kindInfo"] = kinds
	if _, ok := d["updated"]; !ok {
		d["updated"] = time.Now().Format("2006-01-02")
	}
	return nil
}

// load reads feature.json, applies overrides.json from the same folder, validates and derives.
func load(src string) (map[string]any, error) {
	b, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	var raw any
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, DataError{"not valid JSON: " + err.Error()}
	}
	d, ok := raw.(map[string]any)
	if !ok {
		return nil, DataError{"the top level must be an object"}
	}
	ov := readMap(filepath.Join(filepath.Dir(src), "overrides.json"))
	if err := prepare(d, ov); err != nil {
		return nil, err
	}
	return d, nil
}

const historyKeep = 300

// track keeps history.json next to feature.json: what changed between two versions of the file, and when.
// Items are matched by id (stage number + text); an item rewritten in place (same stage, same position)
// is the same item. The first run only takes a snapshot. Returns the changes and the time of the last update.
func track(src string, data map[string]any) ([]any, int64) {
	st, err := os.Stat(src)
	if err != nil {
		return []any{}, 0
	}
	mtime := st.ModTime().UnixMilli()
	path := filepath.Join(filepath.Dir(src), "history.json")
	h := readMap(path)
	changes := []any{}
	if l, ok := h["changes"].([]any); ok {
		for _, c := range l {
			if _, ok := c.(map[string]any); ok {
				changes = append(changes, c)
			}
		}
	}
	snap := map[string]any{}
	for _, x := range data["stages"].([]any) {
		s := x.(map[string]any)
		fids, facts := s["fids"].([]any), s["facts"].([]any)
		pairs := make([]any, len(facts))
		for i, f := range facts {
			pairs[i] = []any{fids[i], f.([]any)[0]}
		}
		snap[fmt.Sprint(s["n"])] = pairs
	}
	old, hasOld := h["snap"].(map[string]any)
	if hasOld && string(encodeJSON(old)) == string(encodeJSON(snap)) {
		return changes, mtime
	}
	when := mtime // a "not needed" decision changes overrides.json only
	if ost, err := os.Stat(filepath.Join(filepath.Dir(src), "overrides.json")); err == nil && ost.ModTime().UnixMilli() > when {
		when = ost.ModTime().UnixMilli()
	}
	if hasOld {
		for _, x := range data["stages"].([]any) {
			s := x.(map[string]any)
			var before [][2]any
			if l, ok := old[fmt.Sprint(s["n"])].([]any); ok {
				for _, p := range l {
					if pl, ok := p.([]any); ok && len(pl) == 2 {
						before = append(before, [2]any{pl[0], pl[1]})
					}
				}
			}
			was := map[string]any{}
			for _, p := range before {
				was[fmt.Sprint(p[0])] = p[1]
			}
			fids, facts := s["fids"].([]any), s["facts"].([]any)
			now := map[string]bool{}
			for _, f := range fids {
				now[f.(string)] = true
			}
			for i, f := range facts {
				fid := fids[i].(string)
				prev, ok := was[fid]
				if !ok && i < len(before) && !now[fmt.Sprint(before[i][0])] {
					gone := fmt.Sprint(before[i][0])
					prev, ok = before[i][1], true // same place, new text: the same item, rewritten
					for _, c := range changes {
						if cm := c.(map[string]any); cm["fid"] == gone {
							cm["fid"] = fid
						}
					}
				}
				k := f.([]any)[0]
				if !ok || prev != k {
					var p any
					if ok {
						p = prev
					}
					changes = append(changes, map[string]any{"ts": when, "fid": fid, "stage": s["n"], "was": p, "now": k, "t": f.([]any)[1]})
				}
			}
		}
	}
	if len(changes) > historyKeep {
		changes = changes[len(changes)-historyKeep:]
	}
	_ = atomicWriteJSON(path, map[string]any{"snap": snap, "changes": changes}) // a read-only folder: the page still renders
	return changes, mtime
}

// blob is JSON that is safe inside a <script> element (Go escapes "<", so nothing can end the script).
func blob(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func page(name string) string {
	b, err := webFS.ReadFile("skill/" + name)
	if err != nil {
		return ""
	}
	return string(b)
}

// panelHTML validates one feature.json and returns the page of the feature. Nothing but history.json is written.
func panelHTML(src, backHref string, live bool) (string, error) {
	html, _, err := panelPage(src, backHref, live)
	return html, err
}

func panelPage(src, backHref string, live bool) (string, map[string]any, error) {
	data, err := load(src)
	if err != nil {
		return "", nil, err
	}
	abs, _ := filepath.Abs(src)
	slug := filepath.Base(filepath.Dir(abs))
	data["live"], data["slug"], data["ver"] = live, slug, featureVersion(slug)
	// the change log is kept only for real features, not for a file rendered from somewhere else
	changes, updated := []any{}, int64(0)
	if filepath.Base(src) == "feature.json" {
		changes, updated = track(src, data)
	}
	if len(changes) > 80 {
		changes = changes[len(changes)-80:]
	}
	data["changes"], data["updatedTs"] = changes, updated
	html := strings.Replace(page("template.html"), "/*__DATA__*/null", blob(data), 1)
	html = strings.Replace(html, "/*__TITLE__*/", strings.ReplaceAll(fmt.Sprint(data["title"]), "<", "&lt;"), 1)
	link := ""
	if backHref != "" {
		link = `<a class="back" href="` + backHref + `">← All features</a>`
	}
	return strings.Replace(html, "<!--__BACK__-->", link, 1), data, nil
}

// summary is one row of the home page, derived from a validated feature.
func summary(slug string, d map[string]any, today time.Time) (row map[string]any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if de, ok := r.(DataError); ok {
				err = de
			} else {
				err = DataError{fmt.Sprint(r)}
			}
		}
	}()
	up, _ := d["updated"].(string)
	t, perr := time.ParseInLocation("2006-01-02", up, time.Local)
	if perr != nil {
		fail(`"updated" must be YYYY-MM-DD, got '%v'`, d["updated"])
	}
	y, m, dd := today.Date()
	days := int(math.Round(time.Date(y, m, dd, 0, 0, 0, 0, time.Local).Sub(t).Hours() / 24))
	blockers := d["blockers"].([]any)
	h, mid, waiting := 0, 0, 0
	var top map[string]any
	for _, sev := range sevs {
		for _, x := range blockers {
			if b := x.(map[string]any); b["sev"] == sev && top == nil {
				top = b
			}
		}
	}
	for _, x := range blockers {
		b := x.(map[string]any)
		switch b["sev"] {
		case "high":
			h++
		case "mid":
			mid++
		}
		if b["who"] == "you" {
			waiting++
		}
	}
	stages := []any{}
	for _, x := range d["stages"].([]any) {
		s := x.(map[string]any)
		stages = append(stages, map[string]any{"n": s["n"], "title": s["title"], "state": s["state"], "kind": s["kind"], "dn": s["dn"], "tot": s["tot"]})
	}
	why, next := any(""), any("")
	if top != nil {
		why = top["t"]
	}
	if ns := d["nextSteps"].([]any); len(ns) > 0 {
		next = ns[0]
	}
	done := d["ready"].(bool)
	return map[string]any{
		"id": slug, "name": d["title"], "projects": d["projects"], "cur": d["cur"], "stages": stages,
		"pct": d["pct"], "h": h, "m": mid, "days": days, "stale": days >= 3 && !done, "done": done,
		"waiting": waiting, "why": why, "next": next, "href": slug + "/panel.html",
	}, nil
}

// collect reads every <root>/<slug>/feature.json: the payload of the home page.
func collect(root string) map[string]any { return collectFiles(root, false) }

// collectFiles with write=true also rewrites each feature's panel.html, for opening the pages without a server.
func collectFiles(root string, write bool) map[string]any {
	today := time.Now()
	feats, broken := []any{}, []any{}
	entries, _ := os.ReadDir(root)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, slug := range names {
		src := filepath.Join(root, slug, "feature.json")
		if !isFile(src) {
			continue
		}
		var data map[string]any
		var err error
		if write {
			data, err = writePanel(src, filepath.Join(root, slug, "panel.html"), "../index.html")
		} else {
			data, err = load(src)
		}
		var row map[string]any
		if err == nil {
			row, err = summary(slug, data, today)
		}
		if err != nil {
			broken = append(broken, map[string]any{"id": slug, "error": err.Error()})
			if write {
				fmt.Fprintf(os.Stderr, "%s: %v\n", slug, err)
			}
			continue
		}
		feats = append(feats, row)
	}
	projects, seen := []any{}, map[string]bool{}
	for _, f := range feats {
		for _, p := range f.(map[string]any)["projects"].([]any) {
			if s := fmt.Sprint(p); !seen[s] {
				seen[s] = true
				projects = append(projects, p)
			}
		}
	}
	return map[string]any{"today": today.Format("2006-01-02"), "projects": projects, "features": feats, "broken": broken, "kinds": kinds}
}

func indexHTML(payload map[string]any) string {
	return strings.Replace(page("template-index.html"), "/*__DATA__*/null", blob(payload), 1)
}
