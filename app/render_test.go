package main

// Ported from tests/test_whatnow.py (FixedFactTest, PlanPercentTest, ProgressTest) and
// tests/test_robustness.py (RenderTest, TemplateEscapeTest).

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func facts(pairs ...string) []any {
	out := []any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, []any{pairs[i], pairs[i+1]})
	}
	return out
}

// ------------------------------------------------------------------ FixedFactTest

func TestFixedDoesNotColourTheStage(t *testing.T) {
	eq(t, stageState(facts("y", "a", "fixed", "b")), "ok")
	eq(t, stageState(facts("fixed", "b", "q", "c")), "part")
	eq(t, stageState(facts("fixed", "b", "bad", "open")), "bad")
	eq(t, stageState(facts("q", "a", "q", "b")), "no")
}

func TestStageColourSaysHowWorriedToBe(t *testing.T) {
	nine := []any{}
	for i := 0; i < 9; i++ {
		nine = append(nine, []any{"y", fmt.Sprint(i)})
	}
	with := func(head []any, kind string) []any { return append(append([]any{}, head...), []any{kind, "x"}) }
	eq(t, stageState(with(nine, "q")), "almost", "90% done")
	eq(t, stageState(with(nine[:4], "q")), "almost", "80% done")
	eq(t, stageState(with(nine[:3], "q")), "part", "75% done")
	eq(t, stageState(with(nine, "bad")), "bad", "an open problem always shows")
}

// RenderTest.test_planned_stage_without_facts_is_not_done
func TestPlannedStageWithoutFactsIsNotDone(t *testing.T) {
	eq(t, stageState([]any{}), "no")
}

// ------------------------------------------------------------------ PlanPercentTest

func planStage(n int, f []any, title string, more ...any) map[string]any {
	s := map[string]any{"n": n, "title": title, "verdict": "", "plan": []any{}, "facts": f}
	for i := 0; i+1 < len(more); i += 2 {
		s[more[i].(string)] = more[i+1]
	}
	return s
}

func prep(t *testing.T, stages []any, more ...any) (map[string]any, error) {
	t.Helper()
	d := map[string]any{"title": "t", "projects": []any{"a"}, "stages": stages}
	for i := 0; i+1 < len(more); i += 2 {
		d[more[i].(string)] = more[i+1]
	}
	m := jsMap(d)
	return m, prepare(m, nil)
}

func mustPrep(t *testing.T, stages []any, more ...any) map[string]any {
	t.Helper()
	d, err := prep(t, stages, more...)
	must(t, err)
	return d
}

func TestEveryStageWeighsTheSame(t *testing.T) {
	d := mustPrep(t, []any{planStage(1, facts("y", "a", "y", "b"), "s"),
		planStage(2, facts("y", "a", "q", "b", "p", "c", "q", "d"), "s"), planStage(3, []any{}, "s")})
	eq(t, d["pct"], any(int(math.Round(100*(1+1.5/4+0)/3))))
	eq(t, d["ready"], any(false))
}

func TestReadyWhenEveryStageIsDoneAndNothingBlocks(t *testing.T) {
	d := mustPrep(t, []any{planStage(1, facts("y", "a"), "s"), planStage(2, facts("fixed", "a"), "s")})
	eq(t, d["pct"], any(100))
	eq(t, d["ready"], any(true))
}

func TestAnOldFileWithALadderStillLoads(t *testing.T) {
	d := mustPrep(t, []any{planStage(1, facts("y", "a"), "s")}, "ladder", map[string]any{"a": "whatever"}, "checks", []any{}, "deploy", []any{})
	eq(t, d["pct"], any(100))
}

func TestKindIsTakenFromTheFileOrReadFromTheTitle(t *testing.T) {
	d := mustPrep(t, []any{planStage(1, []any{}, "Deploy to production"), planStage(2, []any{}, "Код-ревью"),
		planStage(3, []any{}, "Something", "kind", "test"), planStage(4, []any{}, "api: migration, logic, tests"),
		planStage(5, []any{}, "Фаза 4. Документация и аккаунт")})
	var got []string
	for _, s := range d["stages"].([]any) {
		got = append(got, s.(map[string]any)["kind"].(string))
	}
	eqList(t, got, []string{"deploy", "review", "test", "", "docs"})
	_, err := prep(t, []any{planStage(1, []any{}, "s", "kind", "party")})
	var de DataError
	if !errors.As(err, &de) {
		t.Errorf("kind 'party': want a DataError, got %v", err)
	}
}

// ------------------------------------------------------------------ ProgressTest

func progressFeature(stages ...any) map[string]any {
	return map[string]any{"title": "t", "projects": []any{"a"},
		"ladder": map[string]any{"a": []any{"no", "no", "no", "no", "no", "no"}}, "stages": stages}
}

func progStage(n int, f []any) map[string]any {
	return map[string]any{"n": n, "title": fmt.Sprintf("s%d", n), "verdict": "", "plan": []any{}, "facts": f}
}

// renderAt writes the feature with the given mtime (seconds) and renders its page as render.panel_html did.
func renderAt(t *testing.T, src string, d map[string]any, stamp int64) map[string]any {
	t.Helper()
	writeFile(t, src, encodeJSON(d))
	must(t, os.Chtimes(src, time.Unix(stamp, 0), time.Unix(stamp, 0)))
	_, data, err := panelPage(src, "", false)
	must(t, err)
	return data
}

func TestItemInWorkIsNotDoneAndMarksTheCurrentStage(t *testing.T) {
	eq(t, stageState(facts("w", "a", "q", "b")), "part")
	d := jsMap(progressFeature(progStage(1, facts("y", "a")), progStage(2, facts("w", "b", "q", "c")),
		progStage(3, facts("y", "d")), progStage(4, []any{})))
	must(t, prepare(d, nil))
	eq(t, num(d["cur"]), 2, "the item in work wins over a later started stage")
	s := d["stages"].([]any)[1].(map[string]any)
	eq(t, s["dn"], any(0))
	eq(t, s["tot"], any(2))
}

func TestCurrentStageWithoutAnItemInWork(t *testing.T) {
	d := jsMap(progressFeature(progStage(1, facts("y", "a")), progStage(2, facts("p", "b")), progStage(3, []any{})))
	must(t, prepare(d, nil))
	eq(t, num(d["cur"]), 2, "the furthest stage that was started")
	d = jsMap(progressFeature(progStage(1, []any{}), progStage(2, []any{})))
	must(t, prepare(d, nil))
	eq(t, num(d["cur"]), 1)
}

func TestChangeLog(t *testing.T) {
	isolate(t)
	src := filepath.Join(t.TempDir(), "feature.json")
	first := renderAt(t, src, progressFeature(progStage(1, facts("q", "a", "q", "b", "y", "c"))), 1000)
	eq(t, len(first["changes"].([]any)), 0, "the first render only takes a snapshot")
	eq(t, first["updatedTs"], any(int64(1000000)))
	// a -> in work; b rewritten in place and finished; c untouched; d is new
	four := progressFeature(progStage(1, facts("w", "a", "y", "b, done", "y", "c", "bad", "d")))
	data := renderAt(t, src, four, 2000)
	type change struct {
		was, now, t any
		ts          int64
	}
	var got []change
	var fids []any
	for _, c := range data["changes"].([]any) {
		m := c.(map[string]any)
		got = append(got, change{m["was"], m["now"], m["t"], num(m["ts"])})
		fids = append(fids, m["fid"])
	}
	want := []change{{"q", "w", "a", 2000000}, {"q", "y", "b, done", 2000000}, {nil, "bad", "d", 2000000}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("changes: got %v, want %v", got, want)
	}
	sf := data["stages"].([]any)[0].(map[string]any)["fids"].([]any)
	eqList(t, fids, []any{sf[0], sf[1], sf[3]}, "change ids")
	// rendering again without a change adds nothing; a rewrite with the same status keeps the entry on the item
	eq(t, len(renderAt(t, src, four, 2000)["changes"].([]any)), 3)
	data = renderAt(t, src, progressFeature(progStage(1, facts("w", "a", "y", "b, done and checked", "y", "c", "bad", "d"))), 3000)
	ch := data["changes"].([]any)
	eq(t, len(ch), 3)
	eq(t, ch[1].(map[string]any)["fid"], data["stages"].([]any)[0].(map[string]any)["fids"].([]any)[1])
}

func TestAFileThatIsNotAFeatureKeepsNoHistory(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	renderAt(t, filepath.Join(dir, "example.json"), progressFeature(progStage(1, facts("q", "a"))), 1000)
	if _, err := os.Stat(filepath.Join(dir, "history.json")); err == nil {
		t.Error("history.json was written for a file that is not a feature")
	}
}

// ------------------------------------------------------------------ RenderTest (test_robustness.py)

func example(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "examples", "csv-export.json"))
	must(t, err)
	return parseJSON(b).(map[string]any)
}

func rootWith(t *testing.T, data any) string {
	t.Helper()
	root := t.TempDir()
	b, ok := data.(string)
	if !ok {
		b = string(encodeJSON(data))
	}
	writeFile(t, filepath.Join(root, "a", "feature.json"), []byte(b))
	return root
}

func idsOf(l any) []string {
	out := []string{}
	for _, x := range l.([]any) {
		out = append(out, fmt.Sprint(x.(map[string]any)["id"]))
	}
	return out
}

func TestAMalformedFeatureIsReportedNotRaised(t *testing.T) {
	isolate(t)
	changes := map[string]func(d map[string]any){
		"kind":     func(d map[string]any) { d["stages"].([]any)[0].(map[string]any)["kind"] = "party" },
		"projects": func(d map[string]any) { d["projects"] = []any{} },
		"stage":    func(d map[string]any) { d["stages"].([]any)[0] = "oops" },
		"fact":     func(d map[string]any) { d["stages"].([]any)[0].(map[string]any)["facts"] = []any{json.Number("5")} },
	}
	for name, change := range changes {
		d := example(t)
		change(d)
		payload := collect(rootWith(t, d))
		eqList(t, idsOf(payload["features"]), []string{}, name)
		eqList(t, idsOf(payload["broken"]), []string{"a"}, name)
	}
	for _, text := range []string{"{not json", "[1, 2]"} {
		payload := collect(rootWith(t, text))
		eqList(t, idsOf(payload["broken"]), []string{"a"}, text)
	}
}

func TestOneBrokenFeatureDoesNotHideTheOthers(t *testing.T) {
	isolate(t)
	bad := example(t)
	bad["projects"] = []any{}
	root := rootWith(t, bad)
	writeFile(t, filepath.Join(root, "b", "feature.json"), encodeJSON(example(t)))
	payload := collect(root)
	eqList(t, idsOf(payload["features"]), []string{"b"})
	eqList(t, idsOf(payload["broken"]), []string{"a"})
}

func TestDataCannotBreakOutOfTheScriptElement(t *testing.T) {
	isolate(t)
	d := example(t)
	d["summary"] = "</script><script>alert(1)</script><!--<script>"
	root := rootWith(t, d)
	html, err := panelHTML(filepath.Join(root, "a", "feature.json"), "", false)
	must(t, err)
	m := regexp.MustCompile(`(?s)<script>window\.PANEL=(.*?)</script>`).FindStringSubmatch(html)
	if m == nil {
		t.Fatal("no data script in the page")
	}
	if strings.Contains(m[1], "<") {
		t.Errorf("the data script contains '<'")
	}
	back, _ := parseJSON([]byte(strings.TrimRight(strings.TrimSpace(m[1]), ";"))).(map[string]any)
	eq(t, back["summary"], d["summary"])
	if strings.Contains(blob(map[string]any{"broken": []any{map[string]any{"error": "</script>"}}}), "<") {
		t.Error("blob lets '<' through")
	}
}

// ------------------------------------------------------------------ TemplateEscapeTest (needs node)

func rich(t *testing.T, template, text string) string {
	t.Helper()
	fn := regexp.MustCompile(`(?m)^function rich\(t\)\{.*\}$`).FindString(page(template))
	if fn == "" {
		t.Fatalf("%s: no rich() function", template)
	}
	arg, _ := json.Marshal(text)
	out, err := exec.Command(nodeBin, "-e", fn+";process.stdout.write(rich(JSON.parse(process.argv[1])))", string(arg)).Output()
	must(t, err)
	return string(out)
}

func TestOnlyCodeAndBSurvive(t *testing.T) {
	if nodeBin == "" {
		t.Skip("node is not installed")
	}
	for _, tpl := range []string{"template.html", "template-index.html"} {
		eq(t, rich(t, tpl, "run <code>make</code> <b>now</b>"), "run <code>make</code> <b>now</b>", tpl)
		eq(t, rich(t, tpl, "<img src=x onerror=alert(1)>"), "&lt;img src=x onerror=alert(1)&gt;", tpl)
		eq(t, rich(t, tpl, `<b onclick="x()">t</b>`), "&lt;b onclick=&quot;x()&quot;&gt;t</b>", tpl)
		eq(t, rich(t, tpl, "Map<String, Int> & co"), "Map&lt;String, Int&gt; &amp; co", tpl)
		eq(t, rich(t, tpl, "a &lt; b &amp; c"), "a &lt; b &amp; c", tpl)
	}
}

func TestEveryFieldOfTheFeaturePageIsEscaped(t *testing.T) {
	raw := regexp.MustCompile(`'\+(?:[a-zA-Z]+\.(?:t|title|verdict|sub|summary|name|hint|why|todo)|f\[1\]|x|p)\+'`).FindAllString(page("template.html"), -1)
	if len(raw) > 0 {
		t.Errorf("fields put into the page without rich(): %v", raw)
	}
}
