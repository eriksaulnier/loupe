package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
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

// TestPRSettleDropsItsDecisionKeys pins the keys a settling detail drops on a pull request run.
func TestPRSettleDropsItsDecisionKeys(t *testing.T) {
	for _, k := range strings.Split("axsurdew", "") {
		dir := decidedFixture(t)
		m := newModel(t, dir)
		if err := m.openFinding("f-001"); err != nil {
			t.Fatal(err)
		}
		before := loadDraft(t, dir).Version
		m.settling = true
		press(m, k)
		if m.noting || m.editing || m.rewriting || loadDraft(t, dir).Version != before {
			t.Errorf("%s was acted on while settling", k)
		}
	}
	m := newModel(t, decidedFixture(t))
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
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
	if got := fmt.Sprint(publishSteps); got != "[Action Inline comments Message]" {
		t.Errorf("publish steps %s", got)
	}
}
