package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

// The tests here pin specs/038-review-visual-design: the counts and progress the header derives, the notice in the
// pane border, the opener in list and detail, help as an overlay, and the layout at the contract's boundaries.

// mixedFixture is newFixture with f-001 accepted although blocking, f-002 sent back with a reply and a second note,
// and f-003 excluded and then withdrawn by the agent, so its exclusion is current on an unincluded finding.
func mixedFixture(t *testing.T) string {
	t.Helper()
	dir := newFixture(t)
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		if _, err := draft.Accept(d, "f-001", testNow); err != nil {
			return err
		}
		n, err := draft.SendBack(d, "f-002", "Why?", testNow)
		if err != nil {
			return err
		}
		if _, err := draft.AddReply(d, n.ID, "Because.", draft.ByAgent, testNow); err != nil {
			return err
		}
		if _, err := draft.SendBack(d, "f-002", "And this?", testNow); err != nil {
			return err
		}
		_, err = draft.Exclude(d, "f-003", testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[2].Included = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Counts come from the domain derivation: accepted and excluded are decided, a current exclusion on an unincluded
// finding still counts, an open note keeps its finding pending however many replies it has, and blocking counts an
// accepted blocking finding (FR-003, FR-004, FR-006).
func TestHeaderAndBorderCountsFollowTheDerivation(t *testing.T) {
	m := modelOf(t, mixedFixture(t), utf8Env, 100, 30)
	if got := draft.Dispositions(m.draft)["f-003"]; got != draft.DispositionExcluded {
		t.Fatalf("the fixture's f-003 is %s, want excluded", got)
	}
	cells, decided, total := progressOf(m.draft, m.order)
	if decided != 2 || total != 3 || decided > total {
		t.Errorf("decided %d of %d, want 2 of 3", decided, total)
	}
	view := m.View().Content
	if !strings.Contains(view, "2/3 decided") || !strings.Contains(view, "1 blocking · 2 open notes · 1 pending") {
		t.Errorf("header or findings border counts:\n%s", view)
	}
	want := map[string]style.ProgressState{"f-001": style.ProgressAccepted, "f-002": style.ProgressNote, "f-003": style.ProgressExcluded}
	for i, f := range m.order {
		if cells[i] != want[f.ID] {
			t.Errorf("%s has cell %v, want %v", f.ID, cells[i], want[f.ID])
		}
	}

	// A stale accept puts the finding back to pending, and the decided count falls by one.
	if _, err := draft.Mutate(m.cfg.Dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Rev++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if _, decided, total := progressOf(m.draft, m.order); decided != 1 || total != 3 {
		t.Errorf("after a stale accept: %d of %d, want 1 of 3", decided, total)
	}
}

// A withdrawn finding keeps its row and its place in the position, but has no cell and is in neither count.
func TestWithdrawnRowsCountInPositionOnly(t *testing.T) {
	dir := withdrawnFixture(t)
	m := modelOf(t, dir, utf8Env, 100, 30)
	_, decided, total := progressOf(m.draft, m.order)
	rows := listRows(m.View().Content)
	if len(rows) != len(m.order) || total != len(m.order)-1 {
		t.Errorf("%d rows, total %d, for %d findings with one withdrawn", len(rows), total, len(m.order))
	}
	if !strings.Contains(m.View().Content, fmt.Sprintf("%d/%d decided", decided, total)) {
		t.Errorf("count:\n%s", m.View().Content)
	}
	mustOpen(m, m.order[0].ID)
	if !strings.Contains(m.View().Content, fmt.Sprintf(" 1 of %d ", len(m.order))) {
		t.Errorf("position:\n%s", m.View().Content)
	}
}

// emptyFixture is a round with no findings at all.
func emptyFixture(t *testing.T) string {
	t.Helper()
	dir := newFixture(t)
	d := loadDraft(t, dir)
	d.Findings, d.Summary = []draft.Finding{}, ""
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEmptyRoundShowsZeroDecided(t *testing.T) {
	view := modelOf(t, emptyFixture(t), utf8Env, 100, 30).View().Content
	for _, want := range []string{"0/0 decided", "No findings", "No summary"} {
		if !strings.Contains(view, want) {
			t.Errorf("empty round lacks %q:\n%s", want, view)
		}
	}
	if strings.ContainsAny(strings.Split(view, "\n")[0], "=x~!-▮") && strings.Contains(strings.Split(view, "\n")[0], "  -") {
		t.Errorf("an empty round draws a bar:\n%s", view)
	}
}

// Above 30 counted findings the header bar is 20 grouped cells beside the exact count.
func TestLargeRoundGroupsTheBar(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	var many []draft.Finding
	for i := range 31 {
		f := m.draft.Findings[1]
		f.ID = fmt.Sprintf("f-%03d", i+1)
		many = append(many, f)
	}
	m.draft.Findings = many
	m.setDraft(m.draft)
	header := strings.Split(m.View().Content, "\n")[0]
	if !strings.Contains(header, strings.Repeat("-", 20)+"  0/31 decided") || strings.Contains(header, strings.Repeat("-", 21)) {
		t.Errorf("31 findings header %q", header)
	}
}

// A recorded decision is a notice in the findings pane's bottom border that moves no row, and it expires after its
// timer unless a newer notice took its place; a refusal does not expire (FR-010, owner decision 6).
func TestSuccessNoticeLivesInTheBorderAndExpires(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	before := strings.Split(m.View().Content, "\n")
	mustOpenFinding(t, m, "f-001")
	_, cmd := m.Update(runes("a"))
	if cmd == nil {
		t.Fatal("a success notice scheduled no expiry")
	}
	gen := m.noticeGen
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m.say(style.Good, "✓ f-001 accepted")
	after := strings.Split(m.View().Content, "\n")
	if len(after) != len(before) || !strings.Contains(after[len(after)-2], "╰─ ✓ f-001 accepted ─") {
		t.Fatalf("the notice is not in the findings border:\n%s", strings.Join(after, "\n"))
	}
	for i := range before[:len(before)-2] {
		if ansi.Strip(before[i]) != ansi.Strip(after[i]) && !strings.Contains(before[i], "decided") && !strings.Contains(after[i], "f-00") && !strings.Contains(after[i], "blocking") {
			t.Errorf("row %d moved:\n%q\n%q", i, before[i], after[i])
		}
	}
	m.Update(noticeExpired{gen})
	if m.notice == "" {
		t.Error("an older timer cleared a newer notice")
	}
	m.Update(noticeExpired{m.noticeGen})
	if m.notice != "" {
		t.Errorf("the notice's own timer did not clear it: %q", m.notice)
	}
	m.say(style.Warn, "the draft is not ready: pending findings f-002; loupe review")
	if cmd := m.expireNotice(m.noticeGen - 1); cmd != nil {
		t.Error("a refusal was given an expiry")
	}
	if view := m.View().Content; !strings.Contains(paneText(view), "the draft is not ready: pending findings f-002; loupe review") {
		t.Errorf("the refusal is not readable in the pane:\n%s", view)
	}
}

// openerFixture is a list whose target has a pull request URL.
func openerModel(t *testing.T, open func(string) error) *Model {
	t.Helper()
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	m.target.URL = "https://github.com/acme/widgets/pull/42"
	m.cfg.OpenURL = open
	return m
}

// o in list and detail opens the captured pull request, keeps the review running, and on any failure leaves a
// persistent explanation with the URL to copy (FR-013, owner decision 5).
func TestOOpensThePullRequestAndKeepsReviewing(t *testing.T) {
	var opened []string
	m := openerModel(t, func(url string) error { opened = append(opened, url); return nil })
	_, cmd := m.Update(runes("o"))
	if cmd == nil {
		t.Fatal("o started no opener")
	}
	if _, again := m.Update(runes("o")); again != nil || !strings.Contains(m.notice, "already opening") {
		t.Errorf("a second o while opening launched again: notice %q", m.notice)
	}
	_, quit := m.Update(cmd())
	if quit != nil {
		if _, ok := quit().(tea.QuitMsg); ok {
			t.Fatal("a successful open quit the review")
		}
	}
	if len(opened) != 1 || opened[0] != m.target.URL || m.view != viewList || m.browsing {
		t.Fatalf("opened %v, view %v, browsing %v", opened, m.view, m.browsing)
	}

	failing := openerModel(t, func(string) error { return errors.New("no browser") })
	mustOpenFinding(t, failing, "f-001")
	_, cmd = failing.Update(runes("o"))
	failing.Update(cmd())
	if failing.view != viewDetail || !strings.Contains(failing.notice, "Could not open the browser: no browser") ||
		!strings.Contains(failing.notice, "Open this URL manually: https://github.com/acme/widgets/pull/42") {
		t.Fatalf("a failed open: view %v notice %q", failing.view, failing.notice)
	}
	if !strings.Contains(paneConcat(failing.View().Content), "https://github.com/acme/widgets/pull/42") {
		t.Errorf("the URL to copy is not on screen:\n%s", failing.View().Content)
	}

	none := openerModel(t, nil)
	none.Update(runes("o"))
	if !strings.Contains(none.notice, "No browser opener is available. Open this URL manually: https://github.com/acme/widgets/pull/42") {
		t.Errorf("no opener: notice %q", none.notice)
	}
	if footer := strings.Join(none.viewFooter(), " "); !strings.Contains(footer, "o open PR (unavailable)") {
		t.Errorf("no opener: footer %q", footer)
	}

	unsafe := openerModel(t, func(string) error { t.Fatal("an unsafe URL reached the opener"); return nil })
	unsafe.target.URL = "file:///etc/passwd"
	if _, cmd := unsafe.Update(runes("o")); cmd != nil || !strings.Contains(unsafe.notice, "not one loupe will open") {
		t.Errorf("an unsafe URL: notice %q", unsafe.notice)
	}
}

// A result that arrives after the human moved on reaches neither an editor nor the publish flow.
func TestALateOpenerResultLeavesTheNewViewAlone(t *testing.T) {
	m := openerModel(t, func(string) error { return errors.New("late") })
	mustOpenFinding(t, m, "f-001")
	_, cmd := m.Update(runes("o"))
	m.Update(runes("s"))
	m.Update(runes("half a note"))
	m.Update(cmd())
	if !m.noting || m.note.Value() != "half a note" || strings.Contains(m.notice, "late") {
		t.Fatalf("a late result reached the note editor: noting %v value %q notice %q", m.noting, m.note.Value(), m.notice)
	}
}

// Help is an opaque pane over the dimmed screen, which keeps its selection and scroll, and takes the keys while open
// (FR-011).
func TestHelpIsAnOverlayThatKeepsTheScreen(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	m.Update(arrow(tea.KeyDown))
	cursor := m.cursor
	m.Update(runes("?"))
	view := m.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) != 30 || !strings.Contains(ansi.Strip(view), "─ Help ") || !strings.Contains(ansi.Strip(view), "Finding list") {
		t.Fatalf("help is not a pane over the screen:\n%s", ansi.Strip(view))
	}
	if !strings.HasPrefix(ansi.Strip(lines[1]), "╭─ Reviewer") {
		t.Errorf("the covered list is not drawn around the pane:\n%s", ansi.Strip(view))
	}
	if !strings.Contains(view, "38;2;78;80;100") {
		t.Errorf("the covered list is not dimmed:\n%q", view)
	}
	m.Update(arrow(tea.KeyDown))
	m.Update(runes("enter"))
	if m.cursor != cursor || m.view != viewList {
		t.Errorf("a key under help moved the list: cursor %d view %v", m.cursor, m.view)
	}
	m.Update(runes("?"))
	if m.help || m.cursor != cursor {
		t.Errorf("closing help did not return to the same list")
	}
	plain := modelOf(t, newFixture(t), utf8Env, 100, 30)
	plain.Update(runes("?"))
	legend := paneText(strings.Join(plain.helpLines(), "\n"))
	if v := plain.View().Content; strings.ContainsRune(v, '\x1b') || !strings.Contains(v, "╭─ Help ") ||
		!strings.Contains(legend, "Progress: = accepted, x excluded, ~ pending with an open note") || !strings.Contains(legend, "Confidence: ■■■ high") {
		t.Errorf("NO_COLOR help lacks its border, title or legend:\n%s", v)
	}
}

// The contract's boundaries: every screen fills the window exactly at each side of a width or height step, the list
// keeps a finding row and every hint at the minimum, and below it the too-small screen takes over (FR-015).
func TestLayoutBoundaries(t *testing.T) {
	dir := newFixture(t)
	withOpenNote(t, dir)
	for _, size := range [][2]int{{59, 24}, {60, 12}, {60, 11}, {79, 24}, {80, 24}, {80, 23}, {99, 30}, {100, 30}, {100, 29}} {
		for _, sc := range screens {
			m := modelOf(t, dir, utf8Env, size[0], size[1])
			sc.show(m)
			lines := strings.Split(m.View().Content, "\n")
			where := fmt.Sprintf("%dx%d %s", size[0], size[1], sc.name)
			if len(lines) != size[1] {
				t.Errorf("%s: %d rows", where, len(lines))
			}
			for _, l := range lines {
				if style.Width(l) > size[0] {
					t.Errorf("%s: a row is %d cells: %q", where, style.Width(l), l)
				}
			}
			small := strings.Contains(m.View().Content, "Window too small")
			if small != tooSmall(size[0], size[1]) {
				t.Errorf("%s: too-small screen %v", where, small)
			}
		}
	}
	m := modelOf(t, dir, utf8Env, 60, 12)
	view := m.View().Content
	footer := m.viewFooter()
	if len(listRows(view)) < 1 || len(footer) > 3 {
		t.Errorf("60x12 list: %d finding rows, %d footer rows:\n%s", len(listRows(view)), len(footer), view)
	}
	for _, want := range []string{"move", "open", "summary", "open PR", "publish (not ready)", "help", "quit"} {
		if !strings.Contains(strings.Join(footer, " "), want) {
			t.Errorf("60x12 footer lacks %q: %q", want, footer)
		}
	}
}

// A refusal reserves its wrapped rows before the findings are sized, so at the minimum window it stays whole and the
// selected finding still shows (layout contract, notices).
func TestRefusalAtTheMinimumStaysWhole(t *testing.T) {
	var ids []string
	for i := 1; i <= 12; i++ {
		ids = append(ids, fmt.Sprintf("f-%03d", i))
	}
	refusal := "the draft is not ready: pending findings " + strings.Join(ids, ", ") + "; loupe review"
	m := modelOf(t, newFixture(t), utf8Env, 60, 12)
	m.say(style.Warn, refusal)
	view := m.View().Content
	if lines := strings.Split(view, "\n"); len(lines) != 12 {
		t.Fatalf("the list is %d rows at 60x12:\n%s", len(lines), view)
	}
	if !strings.Contains(paneText(view), refusal) {
		t.Errorf("the refusal was cut at 60x12:\n%s", view)
	}
	if !strings.Contains(view, "Title one") {
		t.Errorf("the selected finding is gone at 60x12:\n%s", view)
	}
}

// The detail, file diff and confirmation keep a refusal too long for their pane whole, and the document gives way.
func TestLongRefusalStaysWholeInEveryPane(t *testing.T) {
	// Six wrapped rows: exactly the detail pane at 60x12, so only a document that gives way keeps it whole.
	refusal := strings.Repeat("this refusal runs on and on ", 10) + "and ends with loupe review"
	for _, sc := range []struct {
		name string
		show func(*Model)
	}{
		{"detail", func(m *Model) { mustOpen(m, "f-001"); m.say(style.Warn, refusal) }},
		{"file diff", func(m *Model) {
			mustOpen(m, "f-001")
			f, _ := m.openedFinding()
			m.openFileDiff(f)
			m.say(style.Warn, refusal)
		}},
		{"confirmation", func(m *Model) {
			showConfirmation(m)
			m.confirm.key(m, tea.KeyPressMsg{Code: tea.KeyEsc})
			m.confirm.composeErr = errors.New(refusal)
		}},
	} {
		m := modelOf(t, newFixture(t), utf8Env, 60, 12)
		sc.show(m)
		view := m.View().Content
		if lines := strings.Split(view, "\n"); len(lines) != 12 {
			t.Fatalf("%s is %d rows at 60x12:\n%s", sc.name, len(lines), view)
		}
		if !strings.Contains(paneText(view), refusal) {
			t.Errorf("%s cut the refusal at 60x12:\n%s", sc.name, view)
		}
	}
}

// A refusal taller than the findings pane at the minimum size stays whole: the finding rows give way, and then the
// summary pane (layout contract: no invisible refusal below a clipped viewport).
func TestRefusalTallerThanTheFindingsPaneStaysWhole(t *testing.T) {
	for _, count := range []int{24, 40} {
		var ids []string
		for i := 1; i <= count; i++ {
			ids = append(ids, fmt.Sprintf("f-%03d", i))
		}
		refusal := "the draft is not ready: pending findings " + strings.Join(ids, ", ") + "; loupe review"
		m := modelOf(t, newFixture(t), utf8Env, 60, 12)
		m.say(style.Warn, refusal)
		view := m.View().Content
		if lines := strings.Split(view, "\n"); len(lines) != 12 {
			t.Fatalf("the list is %d rows at 60x12:\n%s", len(lines), view)
		}
		if !strings.Contains(paneText(view), refusal) {
			t.Errorf("a refusal naming %d findings was cut at 60x12:\n%s", count, view)
		}
	}
}

// The Action step keeps its refusal whole at the minimum size, where its explanation shares the choice pane.
func TestActionRefusalStaysWholeAtTheMinimum(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 60, 12)
	m.target.Viewer, m.target.Author = "octocat", "octocat"
	m.view, m.pick = viewAction, slices.Index(publish.Actions, "request-changes")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.notice == "" {
		t.Fatal("request-changes on the viewer's own pull request said nothing")
	}
	view := m.View().Content
	if lines := strings.Split(view, "\n"); len(lines) != 12 {
		t.Fatalf("the Action step is %d rows at 60x12:\n%s", len(lines), view)
	}
	if !strings.Contains(paneText(view), m.notice) {
		t.Errorf("the Action step cut its refusal at 60x12:\n%s", view)
	}
}

// A clipped summary offers the full-summary command whole on one row, in the border when it fits and under the pane
// when it does not, so it can be copied at every width (layout contract, summary budget).
func TestClippedSummaryCommandStaysWhole(t *testing.T) {
	const command = "loupe show --run acme/widgets#42@1"
	for _, width := range []int{60, 79, 100} {
		m := modelOf(t, newFixture(t), utf8Env, width, 14)
		m.draft.Summary = "- one\n- two\n- three\n- four\n- five\n- six"
		l := m.layoutList(len(m.viewFooter()), 0)
		if !l.summaryClipped {
			t.Fatalf("at %d the summary is not clipped: %+v", width, l)
		}
		view := m.View().Content
		lines := strings.Split(view, "\n")
		if len(lines) != 14 {
			t.Fatalf("at %d the list is %d rows:\n%s", width, len(lines), view)
		}
		found := false
		for _, line := range lines {
			if style.Width(line) > width {
				t.Errorf("at %d a row is %d cells: %q", width, style.Width(line), line)
			}
			if strings.Contains(line, command) {
				found = true
			}
		}
		if !found {
			t.Errorf("at %d the full-summary command is not whole on one row:\n%s", width, view)
		}
		if !strings.Contains(view, "Title one") {
			t.Errorf("at %d the selected finding is gone:\n%s", width, view)
		}
	}
}

// A short window collapses the summary on its own without changing the human's choice, which comes back when the
// window grows.
func TestShortWindowCollapsesTheSummaryWithoutForgetting(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 80, 30)
	m.draft.Summary = "- one\n- two\n- three\n- four"
	if l := m.layoutList(len(m.viewFooter()), 0); l.collapsed || len(l.summary) != 4 {
		t.Fatalf("a tall window collapsed the summary: %+v", l)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	if l := m.layoutList(len(m.viewFooter()), 0); !l.collapsed || l.gap != 0 || m.summaryCollapsed {
		t.Fatalf("a short window: %+v, preference collapsed %v", l, m.summaryCollapsed)
	}
	if !strings.Contains(m.View().Content, "collapsed to fit") {
		t.Errorf("an automatic collapse offers tab as if it could expand:\n%s", m.View().Content)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if l := m.layoutList(len(m.viewFooter()), 0); l.collapsed {
		t.Errorf("growing the window did not bring the expanded summary back")
	}
}

// Every nonempty NO_COLOR value turns every escape off, the header link included; 16 colors and the light palette
// still draw, and keep every state's glyph and word.
func TestColorProfilesKeepEveryState(t *testing.T) {
	dir := mixedFixture(t)
	for _, value := range []string{"1", "anything"} {
		m := modelOf(t, dir, map[string]string{"NO_COLOR": value, "LANG": "en_US.UTF-8", "COLORTERM": "truecolor"}, 100, 30)
		m.target.URL = "https://github.com/acme/widgets/pull/42"
		for name, view := range everyView(t, m) {
			if strings.ContainsRune(view, '\x1b') {
				t.Errorf("NO_COLOR=%s %s emits an escape", value, name)
			}
		}
	}
	for _, c := range []struct {
		name    string
		profile colorprofile.Profile
		dark    bool
	}{{"16 colors", colorprofile.ANSI, true}, {"light", colorprofile.TrueColor, false}, {"256 colors", colorprofile.ANSI256, true}} {
		m := modelOf(t, dir, map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
		m.styles.SetColorProfile(c.profile)
		m.styles.SetDarkBackground(c.dark)
		view := m.View().Content
		plain := ansi.Strip(view)
		for _, want := range []string{"✓ f-001", "· f-002", "× f-003", "● ", "✎ ", "2/3 decided"} {
			if !strings.Contains(plain, want) {
				t.Errorf("%s list lacks %q:\n%s", c.name, want, plain)
			}
		}
		if !strings.ContainsRune(view, '\x1b') {
			t.Errorf("%s list paints nothing", c.name)
		}
	}
	dark := modelOf(t, dir, map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	dark.styles.SetColorProfile(colorprofile.TrueColor)
	light := modelOf(t, dir, map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	light.styles.SetColorProfile(colorprofile.TrueColor)
	light.styles.SetDarkBackground(false)
	if dark.View().Content == light.View().Content || !strings.Contains(light.View().Content, "38;2;114;135;253") {
		t.Error("the light palette does not use its own accent")
	}
}

// Long ids, graphemes and large counts fit by display cells in the list and the header.
func TestLongTextFitsByCells(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 80, 24)
	m.target.Title = strings.Repeat("審査", 30)
	m.draft.Findings[0].Title = strings.Repeat("漢字", 40)
	m.draft.Findings[0].ID = "f-100000"
	m.setDraft(m.draft)
	for _, l := range strings.Split(m.View().Content, "\n") {
		if style.Width(l) > 80 {
			t.Errorf("row is %d cells: %q", style.Width(l), l)
		}
	}
	if !strings.Contains(m.View().Content, "f-100000") {
		t.Errorf("a long id was cut:\n%s", m.View().Content)
	}
}

// Tokenizing happens when the displayed lines change, not on every key in the file diff.
func TestFileDiffTokenizesOnce(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	mustOpenFinding(t, m, "f-001")
	f, _ := m.openedFinding()
	m.openFileDiff(f)
	key := m.tokensKey
	start := time.Now()
	for range 20 {
		m.Update(arrow(tea.KeyDown))
	}
	if m.tokensKey != key || time.Since(start) > 2*time.Second {
		t.Errorf("moving the cursor tokenized the file again")
	}
}

// The list shows its position only when the findings overflow the pane; detail always shows it (demo feedback).
func TestListPositionOnlyWhenTheFindingsScroll(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	if view := m.View().Content; strings.Contains(view, "1 of 3") {
		t.Errorf("a list that fits shows its position:\n%s", view)
	}
	mustOpen(m, "f-001")
	if view := m.View().Content; !strings.Contains(view, "1 of 3") {
		t.Errorf("detail lost its position:\n%s", view)
	}
	m = modelOf(t, newFixture(t), utf8Env, 60, 12)
	for i := 4; i <= 12; i++ {
		f := m.order[2]
		f.ID = fmt.Sprintf("f-%03d", i)
		m.draft.Findings = append(m.draft.Findings, f)
		m.order = append(m.order, f)
	}
	if view := m.View().Content; !strings.Contains(view, "1 of 12") {
		t.Errorf("a list that scrolls hides its position:\n%s", view)
	}
}

// The header marks the selected finding's cell in the list and the open finding's in detail; a withdrawn finding has
// no cell to mark (demo feedback).
func TestProgressMarksTheCurrentFinding(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	if got := m.currentCell(); got != 1 {
		t.Errorf("the list marks cell %d, want 1", got)
	}
	first := m.reviewHeader()
	m.Update(arrow(tea.KeyDown))
	if got := m.currentCell(); got != 2 {
		t.Errorf("after down the list marks cell %d, want 2", got)
	}
	if m.reviewHeader() == first {
		t.Error("moving the cursor left the header unchanged")
	}
	mustOpen(m, "f-003")
	if got := m.currentCell(); got != 3 {
		t.Errorf("detail marks cell %d, want 3", got)
	}
	w := modelOf(t, withdrawnFixture(t), utf8Env, 100, 30)
	for i, f := range w.order {
		if draft.Dispositions(w.draft)[f.ID] == draft.DispositionWithdrawn {
			w.cursor = i
			if got := w.currentCell(); got != 0 {
				t.Errorf("a withdrawn finding marks cell %d", got)
			}
		}
	}
}
