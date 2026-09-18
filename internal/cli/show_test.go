package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

func TestPrintShowEscapesDraftAndTargetText(t *testing.T) {
	d := draft.NewEmpty()
	d.Summary = "sum\x1b[2Jmary"
	d.Findings = []draft.Finding{
		{ID: "f-001", Title: "ti\x1b]52;c;x\atle\u202e", Label: "is\u202esue", Location: &draft.Location{Path: "src/\x1bx.go", Line: 3}},
	}
	target := run.Target{Title: "PR\x1b[31m title\u202e"}
	readiness := draft.ReadinessOf(d)
	deps, out := printDeps(t)
	if err := printShow(deps, run.Ref{Owner: "o", Repo: "r", Number: 1, Round: 1}, target, d, draft.Dispositions(d), readiness); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.ContainsAny(got, "\x1b\a\u202e") {
		t.Fatalf("show output carries a raw control:\n%q", got)
	}
	for _, want := range []string{`PR\u001B[31m title\u202E`, `sum\u001B[2Jmary`, `ti\u001B]52;c;x\u0007tle\u202E`, `is\u202Esue`, `src/\u001Bx.go:3`} {
		if !strings.Contains(got, want) {
			t.Errorf("show output lacks %q:\n%s", want, got)
		}
	}
}

func TestShowJSONCarriesDigest(t *testing.T) {
	home := t.TempDir()
	d := draft.NewEmpty()
	d.Summary = "A summary"
	d.Findings = []draft.Finding{{ID: "f-001", Rev: 1, Title: "T", Body: "B", General: true, By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}}}
	draftJSON, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	dir := run.RunDir(home, "o", "r", 1, 1)
	target := run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: 1}
	if err := run.CreateRun(dir, target, nil, draftJSON); err != nil {
		t.Fatal(err)
	}
	stored, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"show", "--run", "o/r#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if got, want := decodeOne(t, s.stdout.Bytes())["digest"], draft.Digest(stored); got != want {
		t.Fatalf("digest %v, want %s", got, want)
	}
}

// diffRun writes one captured run whose pr.diff matches the fingerprint in target.json, and returns the home.
func diffRun(t *testing.T, captured []byte) string {
	t.Helper()
	home := t.TempDir()
	d := draft.NewEmpty()
	d.Version = 3
	draftJSON, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	target := run.Target{
		Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: 1,
		DiffSHA256: run.DiffSHA256(captured),
	}
	if err := run.CreateRun(run.RunDir(home, "o", "r", 1, 1), target, captured, draftJSON); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestShowDiffWritesTheCapturedBytesAlone(t *testing.T) {
	captured := []byte("diff --git a/src/app.go b/src/app.go\n@@ -1,2 +1,2 @@\n-old\n+new\n")
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": diffRun(t, captured)})
	if code := Execute(deps, []string{"show", "--run", "o/r#1", "--diff"}); code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if got := s.stdout.String(); got != string(captured) {
		t.Fatalf("stdout %q, want %q", got, captured)
	}
}

func TestShowDiffJSONCarriesTheDiff(t *testing.T) {
	captured := []byte("diff --git a/src/app.go b/src/app.go\n@@ -1 +1 @@\n-a\n+b\n")
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": diffRun(t, captured)})
	if code := Execute(deps, []string{"show", "--run", "o/r#1", "--diff", "--json"}); code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	got := decodeOne(t, s.stdout.Bytes())
	if got["diff"] != string(captured) {
		t.Fatalf("diff %v, want %q", got["diff"], captured)
	}
	if got["run"] != "o/r#1@1" || got["version"] != float64(3) || got["command"] != "show" {
		t.Fatalf("envelope %v", got)
	}
	if _, ok := got["findings"]; ok {
		t.Fatalf("--diff carried the draft too: %v", got)
	}
}

func TestShowDiffRefusesAChangedDiff(t *testing.T) {
	home := diffRun(t, []byte("diff --git a/src/app.go b/src/app.go\n@@ -1 +1 @@\n-a\n+b\n"))
	path := filepath.Join(run.RunDir(home, "o", "r", 1, 1), "pr.diff")
	if err := os.WriteFile(path, []byte("diff --git a/src/app.go b/src/app.go\n@@ -1 +1 @@\n-a\n+c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"show", "--run", "o/r#1", "--diff"}); code == 0 {
		t.Fatalf("an edited pr.diff was handed over: %q", s.stdout.String())
	}
	if !strings.Contains(s.stderr.String(), "diffSha256") {
		t.Fatalf("stderr %q does not name the fingerprint", s.stderr.String())
	}
}

func TestShowDiffRefusesWithPrevious(t *testing.T) {
	home := diffRun(t, []byte("diff --git a/src/app.go b/src/app.go\n"))
	for _, args := range [][]string{
		{"show", "--run", "o/r#1", "--diff", "--previous"},
		{"show", "--run", "o/r#1", "--diff", "--previous", "--json"},
	} {
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
		if code := Execute(deps, args); code != exitUsage {
			t.Fatalf("%v: exit %d, want %d (stdout %q stderr %q)", args, code, exitUsage, s.stdout.String(), s.stderr.String())
		}
		out := s.stdout.String() + s.stderr.String()
		if !strings.Contains(out, "--diff") || !strings.Contains(out, "--previous") {
			t.Fatalf("%v: refusal %q names neither flag", args, out)
		}
	}
}

// loupe show presents findings in the order the review interface and the published review use.
func TestShowOrdersFindingsBySeverity(t *testing.T) {
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": goldenRuns(t), "NO_COLOR": "1", "LANG": "en_US.UTF-8"})
	deps.TermWidth = func() int { return 100 }
	if code := Execute(deps, []string{"show", "--run", "eriksaulnier/dev-loadout#2"}); code != 0 {
		t.Fatalf("exit %d: %s", code, s.stderr.String())
	}
	got := s.stdout.String()
	var at []int
	for _, id := range []string{"f-002", "f-001", "f-003"} {
		i := strings.Index(got, id)
		if i < 0 {
			t.Fatalf("show never printed %s:\n%s", id, got)
		}
		at = append(at, i)
	}
	if !slices.IsSorted(at) {
		t.Errorf("findings at offsets %v; the rated f-002 must lead:\n%s", at, got)
	}
}

// The severity cell carries its own color instead of sitting in the dim meta line, and the wrapper never splits it.
func TestShowColorsTheSeverityCell(t *testing.T) {
	var out bytes.Buffer
	s := colorStyle(t, &out)
	cells := []metaCell{dimCell("pending"), dimCell("blocking"), dimCell("confidence low"),
		{"severity major", style.Severity("major")}, dimCell("verified plausible"), dimCell("src/cli.ts:598")}

	got := strings.Join(metaLine(s, cells, 100), "\n")
	if want := s.Of(style.Severity("major")).Render("severity major"); !strings.Contains(got, want) {
		t.Errorf("severity cell is not painted by its rank:\n%q", got)
	}
	if strings.Contains(got, s.Dim.Render("severity major")) {
		t.Errorf("severity cell is still dim like the rest of the line:\n%q", got)
	}

	// Every rank paints a different color, and a value captured before the enum stays dim.
	seen := map[string]string{}
	for _, word := range []string{"critical", "major", "minor", "trivial"} {
		seen[s.Of(style.Severity(word)).Render("x")] = word
	}
	if len(seen) != 4 {
		t.Errorf("the four words paint %d colors, not 4: %v", len(seen), seen)
	}
	if style.Severity("P2") != style.Dim || style.Severity("") != style.Dim {
		t.Error("a severity outside the enum must stay dim")
	}

	// At a width that forces a wrap, the cell survives whole on one line.
	for _, line := range metaLine(s, cells, 44) {
		if plain := ansi.Strip(line); strings.HasSuffix(strings.TrimSpace(plain), "severity") || strings.HasPrefix(strings.TrimSpace(plain), "major") {
			t.Errorf("the wrapper split the severity cell:\n%s", strings.Join(metaLine(s, cells, 44), "\n"))
		}
	}
}

// The colored cell has to survive printShow, not only metaLine: the goldens are NO_COLOR, so nothing else would
// notice the call site handing the severity cell the same dim kind as everything beside it.
func TestPrintShowPaintsTheSeverityCellByRank(t *testing.T) {
	d := draft.NewEmpty()
	d.Findings = []draft.Finding{
		{ID: "f-001", Title: "Rated", Body: "B.", General: true, Label: "issue", Confidence: "high", Severity: "critical"},
		{ID: "f-002", Title: "Unrated", Body: "B.", General: true, Label: "issue", Confidence: "high"},
	}
	deps, out := printDeps(t)
	s := colorStyle(t, out)
	deps.palettes = &palettes{out: s, err: s}
	if err := printShow(deps, run.Ref{Owner: "o", Repo: "r", Number: 1, Round: 1}, run.Target{}, d,
		draft.Dispositions(d), draft.ReadinessOf(d)); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if want := s.Of(style.Severity("critical")).Render("severity critical"); !strings.Contains(got, want) {
		t.Errorf("the severity cell is not painted by its rank:\n%q", got)
	}
	if bland := s.Dim.Render("severity critical"); strings.Contains(got, bland) {
		t.Errorf("the severity cell is dim like the rest of the meta line:\n%q", got)
	}
	// Every other cell keeps the dim it had.
	if want := s.Dim.Render("confidence high"); !strings.Contains(got, want) {
		t.Errorf("a plain meta cell lost its dim:\n%q", got)
	}
}

// The separator never ends up alone on a line, and no word that fits is hard-split across two. A cell may still wrap
// at its own space when it is too wide to keep whole; that is the break it always had.
func TestMetaLineNeverOrphansASeparatorOrSplitsAWord(t *testing.T) {
	var buf bytes.Buffer
	s := colorStyle(t, &buf)
	sep := strings.TrimSpace(metaSep(s))
	cells := []metaCell{dimCell("pending"), dimCell("confidence medium"),
		{"severity critical", style.Severity("critical")}, dimCell("verified plausible"),
		dimCell("adapters/pi/skills/post-review/SKILL.md:10")}
	for width := 20; width <= 110; width++ {
		limit := max(10, width-style.Width(findingIndent))
		lines := metaLine(s, cells, width)
		var plain []string
		for _, l := range lines {
			p := strings.TrimSpace(ansi.Strip(l))
			if p == sep {
				t.Fatalf("width %d orphaned the separator:\n%s", width, strings.Join(lines, "\n"))
			}
			plain = append(plain, p)
		}
		for _, c := range cells {
			for _, word := range strings.Fields(c.text) {
				if style.Width(word) > limit {
					continue // A word wider than the line has nowhere to go but across two.
				}
				if !slices.ContainsFunc(plain, func(l string) bool { return strings.Contains(l, word) }) {
					t.Fatalf("width %d split the word %q, which fits in %d:\n%s",
						width, word, limit, strings.Join(lines, "\n"))
				}
			}
		}
	}
}

// A cell short enough to keep whole is kept whole, so its color cannot be lost on a second line.
func TestMetaLineKeepsAShortCellWhole(t *testing.T) {
	var buf bytes.Buffer
	s := colorStyle(t, &buf)
	cells := []metaCell{dimCell("pending"), {"severity critical", style.Severity("critical")}, dimCell("general")}
	for width := 40; width <= 110; width++ {
		joined := strings.Join(metaLine(s, cells, width), "\n")
		if !strings.Contains(ansi.Strip(joined), "severity critical") {
			t.Fatalf("width %d broke a cell that fits:\n%s", width, joined)
		}
	}
}

// A finding with nothing to report on its meta line prints no line at all, not an empty one. Unreachable from the two
// call sites today, since both always append a location cell, and pinned so it stays that way.
func TestWriteFindingWithNoMetaPrintsNoLine(t *testing.T) {
	var b strings.Builder
	var buf bytes.Buffer
	writeFinding(&b, colorStyle(t, &buf), 100, findingBlock{glyph: "·", id: "f-001", title: "T", body: "B."})
	for i, line := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
		if strings.TrimSpace(ansi.Strip(line)) == "" {
			t.Errorf("line %d is blank where the meta line would have been:\n%q", i, b.String())
		}
	}
}

// The no-break marker is a private-use rune, and nothing upstream escapes those: render.ForDisplay hides control, C1
// and bidi runes and leaves U+E000..U+F8FF alone, so a Git path can carry the marker into a cell. It must come out
// the way it went in, the way style.Wrap guards its own marker.
func TestMetaLineKeepsAPrivateUseRuneInACell(t *testing.T) {
	const marker = ""
	var buf bytes.Buffer
	s := colorStyle(t, &buf)
	for _, text := range []string{"src/a" + marker + "b.go:3", marker, "severity P" + marker + "2", "a" + marker + " b"} {
		cells := []metaCell{dimCell("pending"), dimCell(text), dimCell("confidence high")}
		got := ansi.Strip(strings.Join(metaLine(s, cells, 100), "\n"))
		if !strings.Contains(got, text) {
			t.Errorf("cell %q came out as %q", text, strings.TrimSpace(got))
		}
	}
}
