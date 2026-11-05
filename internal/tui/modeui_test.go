package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

// hintText is a footer's hints as key:verb, with the reason an unavailable one cannot run.
func hintText(hints []style.Hint) string {
	var out []string
	for _, h := range hints {
		s := h.Key + ":" + h.Verb
		if h.Unavailable != "" {
			s += "(" + h.Unavailable + ")"
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

// helpText is one view's help section, a row per key.
func helpText(s helpSection) string {
	out := []string{s.title + " [" + s.short + "]"}
	for _, k := range s.keys {
		out = append(out, strings.TrimSpace(k.Keys+" | "+k.Verb+" | "+k.Alias))
	}
	return strings.Join(out, "\n")
}

// decidedFixture is the fixture with f-001 pending under an open note, f-002 accepted and f-003 excluded.
func decidedFixture(t *testing.T) string {
	t.Helper()
	dir := newFixture(t)
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		if _, err := draft.SendBack(d, "f-001", "Why?", testNow); err != nil {
			return err
		}
		if _, err := draft.Accept(d, "f-002", testNow); err != nil {
			return err
		}
		_, err := draft.Exclude(d, "f-003", testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestPRReviewTablesStayAsTheyWere pins what a pull request run offers, so moving the tables behind a mode UI
// changes none of it.
func TestPRReviewTablesStayAsTheyWere(t *testing.T) {
	m := modelOf(t, decidedFixture(t), utf8Env, 100, 40)
	if got, want := hintText(m.listHints()), "↑↓:move enter:open tab:summary o:open PR(unavailable) p:publish(not ready) ?:help q:quit"; got != want {
		t.Errorf("list hints\n got %s\nwant %s", got, want)
	}
	for _, c := range []struct{ id, want string }{
		{"f-001", "←→:finding ↑↓:scroll a:accept x:exclude s:send back e:edit w:rewrite r:resolve d:dismiss f:file esc:list o:open PR(unavailable) ?:help"},
		{"f-002", "←→:finding ↑↓:scroll x:exclude s:send back e:edit w:rewrite f:file esc:list o:open PR(unavailable) ?:help"},
		{"f-003", "←→:finding ↑↓:scroll u:restore e:edit w:rewrite esc:list o:open PR(unavailable) ?:help"},
	} {
		if err := m.openFinding(c.id); err != nil {
			t.Fatal(err)
		}
		if got := hintText(m.detailHints()); got != c.want {
			t.Errorf("%s detail hints\n got %s\nwant %s", c.id, got, c.want)
		}
	}
	w := modelOf(t, withdrawnFixture(t), utf8Env, 100, 40)
	if err := w.openFinding("f-002"); err != nil {
		t.Fatal(err)
	}
	if got, want := hintText(w.detailHints()), "←→:finding ↑↓:scroll u:reinstate r:resolve d:dismiss f:file esc:list o:open PR(unavailable) ?:help"; got != want {
		t.Errorf("withdrawn detail hints\n got %s\nwant %s", got, want)
	}

	sections := m.helpSections()
	wantHelp := map[view]string{
		viewList: `Finding list [list]
↑/↓ | move | k/j
enter | open the finding |
tab | expand or collapse the summary |
o | open the pull request in the browser |
p | publish once the review is ready |
ctrl+r | reload the draft from disk |`,
		viewDetail: `Finding detail [detail]
←/→ | previous / next finding | N/n
↑/↓ | scroll the finding | k/j
PgUp/PgDn | scroll one page |
space | scroll one page down |
a | accept; resolves its open notes |
x | exclude; dismisses its open notes |
s | send back; enter sends, esc cancels |
e | edit label and blocking |
w | rewrite title and body; ctrl+s saves, esc keeps the text |
u | restore or reinstate the finding |
r / d | resolve / dismiss its open note |
f | whole-file diff |
o | open the pull request in the browser |
ctrl+r | reload the draft from disk |
esc | back to the list |`,
		viewAction: `Publish steps [publish]
↑/↓ | move | k/j
enter | choose and go on |
esc | back one step |`,
		viewFileDiff: `File diff [file diff]
↑/↓ | move one line | k/j
←/→ | previous / next marker | [/]
enter | open the finding on this line |
esc | back to the finding |`,
	}
	if len(sections) != len(wantHelp) {
		t.Errorf("help has %d sections, want %d", len(sections), len(wantHelp))
	}
	for v, want := range wantHelp {
		if got := helpText(sections[v]); got != want {
			t.Errorf("help for view %d\n got %s\nwant %s", v, got, want)
		}
	}
}

// TestSettleDropsTheModesDecisionKeys pins the keys a settling detail drops on each kind of run. f decides nothing,
// so it still opens the file diff.
func TestSettleDropsTheModesDecisionKeys(t *testing.T) {
	for _, c := range []struct {
		name, keys string
		open       func(t *testing.T) (*Model, string)
	}{
		{"pull request", "axsurdew", func(t *testing.T) (*Model, string) {
			dir := decidedFixture(t)
			return newModel(t, dir), dir
		}},
		{"local", "aAxXsurd", func(t *testing.T) (*Model, string) { return applyModel(t, "Claude") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, k := range strings.Split(c.keys, "") {
				m, dir := c.open(t)
				mustOpenFinding(t, m, "f-001")
				before := loadDraft(t, dir).Version
				m.settling = true
				press(m, k)
				if m.noting || m.editing || m.rewriting || loadDraft(t, dir).Version != before {
					t.Errorf("%s was acted on while settling", k)
				}
			}
		})
	}
	m := newModel(t, decidedFixture(t))
	mustOpenFinding(t, m, "f-001")
	m.settling = true
	press(m, "f")
	if m.view != viewFileDiff {
		t.Error("f was dropped while settling; only decision keys are")
	}
}

// TestPRKeysOPEAndX pins what o, p, e and x do on a pull request run.
func TestPRKeysOPEAndX(t *testing.T) {
	m := newModel(t, decidedFixture(t))
	press(m, "o")
	if !strings.Contains(m.notice, "The pull request URL is not one loupe will open") {
		t.Errorf("o in the list: notice %q", m.notice)
	}
	press(m, "p")
	if m.view != viewList || !strings.Contains(m.notice, "the draft is not ready") {
		t.Errorf("p on a draft not ready: view %d notice %q", m.view, m.notice)
	}

	dir := newFixture(t)
	m = newModel(t, dir)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	press(m, "e")
	if !m.editing {
		t.Error("e did not open the label and blocking editor")
	}
	m.Update(escKey)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	press(m, "o")
	if !strings.Contains(m.notice, "The pull request URL is not one loupe will open") {
		t.Errorf("o in the detail: notice %q", m.notice)
	}
	press(m, "x")
	d := loadDraft(t, dir)
	if got := draft.Dispositions(d)["f-001"]; got != draft.DispositionExcluded || d.Decisions["f-001"].Note != "" {
		t.Errorf("x: f-001 is %s with note %q, want excluded with none", got, d.Decisions["f-001"].Note)
	}
	if m.noting {
		t.Error("x opened an input on a pull request run")
	}

	ready := newModel(t, readyFixture(t, "", ""))
	press(ready, "p")
	if ready.view != viewAction {
		t.Errorf("p on a ready draft: view %d, want the action step", ready.view)
	}
}

// applyInputs are a local run's findings: f-001 at a.go:3 with a proposal, f-002 a general question with none, and
// f-003 general with a proposal.
var applyInputs = []draft.FindingInput{
	{Title: "Rename the helper", Body: "The helper name hides what it does.", Location: &draft.LocationInput{Path: "a.go", Line: 3}, Label: "issue",
		Proposal: "Rename helper to insertAfter in a.go."},
	{Title: "Which store wins", Body: "Two stores could hold the cache.", General: true, Label: "question"},
	{Title: "Drop the flag", Body: "The flag is never read.", General: true, Label: "suggestion", Proposal: "Remove the flag."},
}

// applyFixture writes a local run captured with --changed over a new ten-line a.go, holding inputs, and applied by
// applier.
func applyFixture(t *testing.T, applier string, inputs ...draft.FindingInput) string {
	t.Helper()
	return localFixture(t, "changed", applier, inputs...)
}

// newFileDiff adds a.go, ten lines long.
func newFileDiff() []byte {
	var b strings.Builder
	b.WriteString("diff --git a/a.go b/a.go\nnew file mode 100644\n--- /dev/null\n+++ b/a.go\n@@ -0,0 +1,10 @@\n")
	for i := range 10 {
		fmt.Fprintf(&b, "+line %d\n", i+1)
	}
	return []byte(b.String())
}

// localFixture is applyFixture with the capture flag named: changed or files.
func localFixture(t *testing.T, capture, applier string, inputs ...draft.FindingInput) string {
	t.Helper()
	diffBytes := newFileDiff()
	parsed, err := diff.Parse(diffBytes)
	if err != nil {
		t.Fatal(err)
	}
	d := draft.NewEmpty()
	if _, err := draft.Add(d, inputs, parsed, draft.Rules{AllowProposal: true}, draft.ByAgent, testNow); err != nil {
		t.Fatal(err)
	}
	d.Summary = "Local work to check."
	draftJSON, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	target := run.Target{
		Kind: run.KindLocal, Title: "Cache rework", Applier: applier, CapturedAt: testNow, DiffSHA256: run.DiffSHA256(diffBytes),
		Local: &run.LocalTarget{Prefix: "acme/widgets", Repository: "acme/widgets", Capture: capture, Root: "/src", Branch: "feat/cache",
			Files: []run.LocalFile{{Path: "a.go", Abs: "/src/a.go", New: true}}, Skipped: []run.LocalSkipped{}},
	}
	dir, _, err := run.NextLocal(t.TempDir(), "test", envOf(nil), target, diffBytes, draftJSON, nil)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func applyModel(t *testing.T, applier string) (*Model, string) {
	t.Helper()
	dir := applyFixture(t, applier, applyInputs...)
	return modelOf(t, dir, utf8Env, 100, 40), dir
}

// keysOf is the keys of hints, in order.
func keysOf(hints []style.Hint) []string {
	var out []string
	for _, h := range hints {
		out = append(out, h.Key)
	}
	return out
}

// On a local run the detail offers each decision key next to its shift, which adds the human's words: on an accepted
// finding A, x and X stay. TestApplyVerbsAreAimedAtTheAgent pins the pending finding's hints.
func TestApplyDetailPairsEachDecisionWithItsShift(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	mustOpenFinding(t, m, "f-001")
	outside(t, dir, accepted("f-001"))
	if _, err := m.reload(); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(keysOf(m.detailHints()), " "), "A x X s"; !strings.Contains(got, want) {
		t.Errorf("accepted detail keys %q lack %q", got, want)
	}
}

// The header of a local run carries the apply badge as words at every icon tier, and a pull request run's carries none.
func TestApplyHeaderShowsTheBadge(t *testing.T) {
	dir := applyFixture(t, "Claude", applyInputs...)
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8", style.IconsEnv: "nerd"}, "[local · returns to Claude]"},
		{utf8Env, "[local · returns to Claude]"},
		{map[string]string{"NO_COLOR": "1", "LANG": "C"}, "[local -> Claude]"},
	} {
		m := modelOf(t, dir, c.env, 124, 30)
		for _, show := range []func(){func() {}, func() { press(m, "l") }, func() { mustOpenFinding(t, m, "f-001") }} {
			show()
			if header := ansi.Strip(m.reviewHeader()); !strings.Contains(header, c.want) {
				t.Errorf("tier %v view %d: header %q lacks %q", m.glyphs.Tier, m.view, header, c.want)
			}
		}
	}
	nobody := modelOf(t, applyFixture(t, "", applyInputs...), utf8Env, 124, 30)
	if header := ansi.Strip(nobody.reviewHeader()); !strings.Contains(header, "[local · returns to the agent]") {
		t.Errorf("with no applier: header %q", header)
	}
	narrow := modelOf(t, dir, utf8Env, 60, 20)
	header := ansi.Strip(narrow.reviewHeader())
	if !strings.Contains(header, "feat/cache") || !strings.Contains(header, "[local") || strings.Contains(header, "returns to") {
		t.Errorf("at 60 columns the badge did not shorten first: %q", header)
	}
	colored := colorOf(t, dir)
	colored.Update(tea.WindowSizeMsg{Width: 124, Height: 40})
	if view := colored.reviewHeader(); !strings.Contains(view, colored.styles.ApplyBadge("local · returns to Claude")) || strings.Contains(ansi.Strip(view), "[local") {
		t.Errorf("the colored badge is not drawn in the apply token: %q", view)
	}
	for _, env := range []map[string]string{utf8Env, {"NO_COLOR": "1", "LANG": "C"}} {
		pr := modelOf(t, newFixture(t), env, 100, 30)
		if header := ansi.Strip(pr.reviewHeader()); strings.Contains(header, "local") {
			t.Errorf("a pull request run's header has the badge: %q", header)
		}
	}
}

// A local run's footer, help and verbs are aimed at the agent: accept, tweak, reject, ask, and return to the applier.
func TestApplyVerbsAreAimedAtTheAgent(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	press(m, "l")
	if got, want := hintText(m.listHints()), "↑↓:move enter:open tab:summary p:return to Claude(not ready) ?:help q:quit"; got != want {
		t.Errorf("list hints\n got %s\nwant %s", got, want)
	}
	mustOpenFinding(t, m, "f-001")
	if got, want := hintText(m.detailHints()), "←→:finding ↑↓:scroll a:accept A:tweak x:reject X:reject with reason s:ask f:file l:list ?:help"; got != want {
		t.Errorf("detail hints\n got %s\nwant %s", got, want)
	}
	want := `Finding detail [detail]
←/→ | previous / next finding | N/n
↑/↓ | scroll the finding | k/j
PgUp/PgDn | scroll one page |
space | scroll one page down |
a | accept; resolves its open notes |
A | tweak: accept with a note; enter accepts, esc cancels |
x | reject; dismisses its open notes |
X | reject with a reason; enter rejects, esc cancels |
s | ask: send back with a question; enter sends, esc cancels |
u | restore or reinstate the finding |
r / d | resolve / dismiss its open note |
f | whole-file diff |
ctrl+r | reload the draft from disk |
l / esc | back to the list |`
	if got := helpText(m.helpSections()[viewDetail]); got != want {
		t.Errorf("detail help\n got %s\nwant %s", got, want)
	}
}

// A local finding reads in the fixed order: location, Why, then what the applier will do. A question takes that slot
// with no glyph and no will section (docs/tui-design.md, Order).
func TestApplyDetailLeadsWithWhatTheApplierWillDo(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	mustOpenFinding(t, m, "f-001")
	screen := screenText(m)
	order := []string{"Rename the helper", "a.go:3", "Why", "The helper name hides what it does.", "Claude will", "Rename helper to insertAfter in a.go."}
	at := -1
	for _, want := range order {
		i := strings.Index(screen, want)
		if i <= at {
			t.Fatalf("%q is missing or out of order (want %q):\n%s", want, order, screen)
		}
		at = i
	}
	mustOpenFinding(t, m, "f-002")
	screen = screenText(m)
	if strings.Contains(screen, "will") || !strings.Contains(screen, "│ A question for Claude") || strings.Contains(screen, m.glyphs.Question+" A question") {
		t.Errorf("a question shows a will section, no question section, or a glyph:\n%s", screen)
	}
	nobody, _ := applyModel(t, "")
	mustOpenFinding(t, nobody, "f-003")
	if screen := screenText(nobody); !strings.Contains(screen, "│ The agent will ") {
		t.Errorf("with no applier the lead does not name the agent:\n%s", screen)
	}
	pr := modelOf(t, newFixture(t), utf8Env, 100, 40)
	mustOpenFinding(t, pr, "f-003")
	if screen := screenText(pr); strings.Contains(screen, "A question for") {
		t.Errorf("a pull request run marks a question:\n%s", screen)
	}
}

// On a local run the notices and note panes use the agent-facing verbs of the footer: reject, ask and tweak.
func TestApplyNoticesUseTheAgentVerbs(t *testing.T) {
	dir := localFixture(t, "changed", "Claude", applyInputs...)
	m := modelOf(t, dir, utf8Env, 100, 40)
	mustOpenFinding(t, m, "f-001")
	press(m, "x")
	if !strings.Contains(m.notice, "f-001 rejected") || strings.Contains(m.notice, "excluded") {
		t.Errorf("x notice %q", m.notice)
	}
	mustOpenFinding(t, m, "f-002")
	press(m, "A")
	if view := screenText(m); !strings.Contains(view, "Tweak f-002") {
		t.Errorf("A pane title:\n%s", view)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	press(m, "s")
	if view := screenText(m); !strings.Contains(view, "Ask about f-002") || strings.Contains(view, "Send back f-002") {
		t.Errorf("s pane title:\n%s", view)
	}
	typeText(m, "Which cache?")
	press(m, "enter")
	if !strings.Contains(m.notice, "f-002 asked as n-") || !strings.Contains(m.notice, "Claude has it") {
		t.Errorf("s notice %q", m.notice)
	}
}

// A local header's place takes the repository from the prefix, or the root without one, and names each capture form
// (docs/tui-design.md, The header).
func TestLocalPlace(t *testing.T) {
	two := []run.LocalFile{{Path: "a.go"}, {Path: "b.go"}}
	for _, c := range []struct {
		local run.LocalTarget
		want  style.LocalPlace
	}{
		{run.LocalTarget{Prefix: "acme/widgets", Branch: "feat/cache", Capture: "files", Files: two}, style.LocalPlace{Repo: "widgets", Branch: "feat/cache", Capture: "2 files"}},
		{run.LocalTarget{Root: "/src/widgets", Capture: "changed", BaseRef: "main"}, style.LocalPlace{Repo: "widgets", Capture: "changes since main"}},
		{run.LocalTarget{Prefix: "acme/widgets", Capture: "changed"}, style.LocalPlace{Repo: "widgets", Capture: "uncommitted changes"}},
		{run.LocalTarget{Prefix: "acme/widgets", Capture: "no-files"}, style.LocalPlace{Repo: "widgets", Capture: "no files"}},
	} {
		if got := *localPlace(run.Target{Local: &c.local}); got != c.want {
			t.Errorf("%+v: place %+v, want %+v", c.local, got, c.want)
		}
	}
	if got := *localPlace(run.Target{}); got != (style.LocalPlace{}) {
		t.Errorf("a target with no local block: place %+v", got)
	}
}

// A word a local run has no form of reads as on a pull request run.
func TestApplyKeepsAWordItHasNoFormOf(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	for word, want := range map[string]string{"excluded": "rejected", "the agent": "Claude", "accepted": "accepted"} {
		if got := m.ui.word(m, word); got != want {
			t.Errorf("%q reads %q, want %q", word, got, want)
		}
	}
}
