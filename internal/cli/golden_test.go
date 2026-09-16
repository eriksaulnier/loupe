package cli

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"

	"github.com/eriksaulnier/loupe/internal/style"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/golden/cli from the renderers")

// goldenRuns is one home with two runs: one mid-review with a send-back, a reply and a blocking finding, and one that
// has nothing filed against it yet, so every state a human view can show appears in the goldens.
func goldenRuns(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	at := time.Date(2026, 9, 11, 20, 13, 38, 0, time.UTC)

	d := draft.NewEmpty()
	d.Version = 8
	d.Summary = "This is a large documentation and contract change (canonical `review-*` skills plus `gadfly-review-*` compatibility shims, an `unable` verifier verdict, evidence-preservation rules, publication moved wholly into `post-review`) around one small code change."
	d.Findings = []draft.Finding{
		{ID: "f-001", Rev: 1, Title: "Alias metadata removed from three skills, still live in four others",
			Body:     "This change removes `metadata.aliases` from three skills and justifies it in both READMEs. By that premise, the four skills that still declare it are dead configuration whose advertised names never resolve.",
			Location: &draft.Location{Path: "adapters/pi/skills/post-review/SKILL.md", Side: draft.SideRight, Line: 10},
			Label:    "issue", Confidence: "medium", By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}},
		{ID: "f-002", Rev: 2, Title: `A packaged gh build reports as "unavailable" rather than as a version`,
			Body:         "The regex requires whitespace or end-of-line immediately after the patch number, so a packaged build with a suffix is reported as unavailable.",
			SuggestedFix: "const m = /^gh version (\\d+\\.\\d+\\.\\d+)/.exec(line);",
			Location:     &draft.Location{Path: "src/cli.ts", Side: draft.SideRight, Line: 598},
			Label:        "suggestion", Blocking: true, Confidence: "low", Severity: "major", Verified: "plausible",
			Impact:     "Every packaged build is reported as unavailable, so the version gate never passes on a release machine.",
			References: []string{"https://github.com/cli/cli/issues/1"},
			By:         draft.ByAgent, Included: true, History: []draft.HistoryEntry{}},
		{ID: "f-003", Rev: 1, Title: "Authority clause quotes a README phrase to satisfy a substring test",
			Body: "The clause exists to pass the test, not to state a rule.", General: true,
			By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}},
	}
	if _, err := draft.Accept(d, "f-001", at); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Exclude(d, "f-003", at); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.SendBack(d, "f-002", "Please run gh --version on a Debian box and paste the output.", at); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.AddReply(d, "n-001", "Ran it on a fresh Debian 12 container: `gh version 2.63.0 (2024-12-05)`, no suffix. Lowered the finding to confidence low and kept it.", draft.ByAgent, at); err != nil {
		t.Fatal(err)
	}
	d.Version = 11
	writeRun(t, home, "eriksaulnier", "dev-loadout", 2, 1, "chore: canonical review skills, gh doctor check, plannotator bump", at, d)
	writeRun(t, home, "eriksaulnier", "loupe-probe", 1, 1, "probe", at.Add(-48*time.Hour), draft.NewEmpty())
	return home
}

func writeRun(t *testing.T, home, owner, repo string, number, round int, title string, at time.Time, d *draft.Draft) {
	t.Helper()
	draftJSON, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	target := run.Target{Schema: run.TargetSchema, Owner: owner, Repo: repo, Number: number, Round: round, Title: title, CapturedAt: at}
	if err := run.CreateRun(run.RunDir(home, owner, repo, number, round), target, nil, draftJSON); err != nil {
		t.Fatal(err)
	}
}

// TestHumanOutputGoldens pins what a human sees at the two widths that matter, with color off, so a change to any
// view shows up as a diff of the screen instead of a diff of format strings.
func TestHumanOutputGoldens(t *testing.T) {
	home := goldenRuns(t)
	cases := []struct {
		name string
		args []string
	}{
		{"help", []string{"--help"}},
		{"handoff-help", []string{"handoff", "--help"}},
		{"publish-help", []string{"publish", "--help"}},
		{"list", []string{"list"}},
		{"show", []string{"show", "--run", "eriksaulnier/dev-loadout#2"}},
		{"feedback", []string{"feedback", "--run", "eriksaulnier/dev-loadout#2"}},
		{"refusal", []string{"show", "--run", "eriksaulnier/dev-loadout#404"}},
	}
	for _, width := range []int{100, 80} {
		for _, c := range cases {
			t.Run(c.name+"."+strconv.Itoa(width), func(t *testing.T) {
				deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1", "LANG": "en_US.UTF-8"})
				deps.TermWidth = func() int { return width }
				deps.Now = func() time.Time { return time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC) }
				Execute(deps, c.args)
				got := s.stdout.String() + s.stderr.String()
				if strings.Contains(got, "\x1b") {
					t.Fatalf("NO_COLOR output carries an escape sequence:\n%q", got)
				}
				checkCLIGolden(t, c.name+"."+strconv.Itoa(width)+".txt", got)
			})
		}
	}
}

// TestNerdTierGolden pins the list under the default tier: the icons as escapes, one space after each, no segment
// joints without color.
func TestNerdTierGolden(t *testing.T) {
	home := goldenRuns(t)
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1", "LANG": "en_US.UTF-8", style.IconsEnv: "nerd"})
	deps.TermWidth = func() int { return 100 }
	deps.Now = func() time.Time { return time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC) }
	Execute(deps, []string{"list"})
	got := s.stdout.String() + s.stderr.String()
	if !strings.Contains(got, "\uf407 eriksaulnier/dev-loadout#2") || !strings.Contains(got, "\uf00c 1 \uf10c 1") || strings.Contains(got, "\ue0b0") {
		t.Fatalf("nerd list:\n%s", got)
	}
	checkCLIGolden(t, "list.nerd.txt", got)
}

func checkCLIGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", "cli", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestASCIIGlyphsUnderANonUTF8Locale(t *testing.T) {
	home := goldenRuns(t)
	for _, args := range [][]string{{"list"}, {"show", "--run", "eriksaulnier/dev-loadout#2"}, {"feedback", "--run", "eriksaulnier/dev-loadout#2"}} {
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1", "LC_ALL": "C"})
		deps.TermWidth = func() int { return 100 }
		if code := Execute(deps, args); code != 0 {
			t.Fatalf("%v exit %d stderr %q", args, code, s.stderr.String())
		}
		got := s.stdout.String()
		for _, glyph := range []string{"✓", "·", "✗", "↩", "●", "✎", "↳", "┃", "…"} {
			if strings.Contains(got, glyph) {
				t.Errorf("%v printed %q under LC_ALL=C:\n%s", args, glyph, got)
			}
		}
	}
}
