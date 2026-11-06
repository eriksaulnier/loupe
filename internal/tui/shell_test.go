package tui

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/style"
)

// With the clip on, as outside tests, a line wider than the window is cut to it and a line that fits is kept.
func TestClipCutsALineWiderThanTheWindow(t *testing.T) {
	clipToWindow = true
	t.Cleanup(func() { clipToWindow = false })
	m := newModel(t, newFixture(t))
	if got := m.clip(strings.Repeat("x", 150)); got != strings.Repeat("x", 100) {
		t.Errorf("a 150-cell line clipped to %d cells", style.Width(got))
	}
	if got := m.clip("short"); got != "short" {
		t.Errorf("a line that fits became %q", got)
	}
}

// A notice taller than its rows keeps its last row, which carries the fix, then its first; with no rows it is gone.
func TestShortenNoticeKeepsTheEndsAtEveryHeight(t *testing.T) {
	m := newModel(t, newFixture(t))
	notice := []string{"first", "second", "third", "fix"}
	for _, c := range []struct {
		rows int
		want []string
	}{
		{0, nil},
		{-1, nil},
		{1, []string{"fix"}},
		{2, []string{"first", "fix"}},
		{3, []string{"first", "... 2 more rows", "fix"}},
	} {
		if got := m.shortenNotice(notice, c.rows); !slices.Equal(got, c.want) {
			t.Errorf("%d rows: %q, want %q", c.rows, got, c.want)
		}
	}
}

// A recorded decision is a notice at the right of the findings pane's top border, before its counts, that moves no
// row, and it expires after its timer unless a newer notice took its place; a refusal does not expire (FR-010, owner
// decision 6, release testing).
func TestSuccessNoticeLivesInTheBorderAndExpires(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	before := strings.Split(m.View().Content, "\n")
	mustOpenFinding(t, m, "f-001")
	// The decision's command also carries the flash and the settle timers, so the expiry is looked for among them.
	expiry, flash, settle := noticeExpiry, flashFor, settleAfterDecision
	t.Cleanup(func() { noticeExpiry, flashFor, settleAfterDecision = expiry, flash, settle })
	noticeExpiry, flashFor, settleAfterDecision = time.Millisecond, time.Millisecond, time.Millisecond
	_, cmd := m.Update(runes("a"))
	gen := m.noticeGen
	if !slices.Contains(drain(cmd), tea.Msg(noticeExpired{gen})) {
		t.Fatal("a success notice scheduled no expiry")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m.say(style.Good, "✓ f-001 accepted")
	after := strings.Split(m.View().Content, "\n")
	top := regexp.MustCompile(`(?m)^╭─ Findings ─+ ✓ f-001 accepted ─ 1 blocking · 0 open notes · 2 pending ─╮$`)
	if len(after) != len(before) || !top.MatchString(m.View().Content) || strings.Contains(after[len(after)-2], "accepted") {
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

// In detail the notice sits at the right end of the finding's top border, apart from the id it does not name, and the
// bottom border keeps only the position (release testing).
func TestDetailNoticeLivesInTheTopBorder(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	mustOpenFinding(t, m, "f-002")
	m.say(style.Good, "✓ f-001 accepted")
	view := m.View().Content
	if !regexp.MustCompile(`(?m)^╭─ · f-002 · pending ─+ ✓ f-001 accepted ─╮$`).MatchString(view) ||
		!regexp.MustCompile(`(?m)^╰─+ 2 of 3 ─╯$`).MatchString(view) {
		t.Errorf("the notice is not at the right of the detail's top border:\n%s", view)
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
	if !strings.HasPrefix(ansi.Strip(lines[bodyTop(30)]), "╭─ Reviewer") {
		t.Errorf("the covered list is not drawn around the pane:\n%s", ansi.Strip(view))
	}
	if muted := sgrOf(t, m.styles.Muted("x")); !strings.Contains(view, muted) {
		t.Errorf("the covered list is not dimmed in %q:\n%q", muted, view)
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

// A refusal reserves its wrapped rows before the findings are sized, so at the minimum window it stays whole and the
// selected finding still shows (layout contract, notices). One taller than the findings pane stays whole too: the
// finding rows give way, and then the summary pane (layout contract: no invisible refusal below a clipped viewport).
func TestRefusalAtTheMinimumStaysWhole(t *testing.T) {
	for _, c := range []struct {
		count       int
		findingStay bool
	}{{12, true}, {24, false}, {40, false}} {
		var ids []string
		for i := 1; i <= c.count; i++ {
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
			t.Errorf("a refusal naming %d findings was cut at 60x12:\n%s", c.count, view)
		}
		if c.findingStay && !strings.Contains(view, "Title one") {
			t.Errorf("the selected finding is gone at 60x12:\n%s", view)
		}
	}
}

// The detail and file diff keep a refusal too long for their pane whole, and the document gives way.
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
	// The confirmation's pane also holds which keys answer after a stray key, and the refusal's ends and that line stay.
	m := modelOf(t, newFixture(t), utf8Env, 60, 12)
	showConfirmation(m)
	m.confirm.key(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m.confirm.key(m, runes("z"))
	m.confirm.composeErr = errors.New(refusal)
	view := m.View().Content
	text := paneText(view)
	if !strings.Contains(text, "this refusal runs on and on") || !strings.Contains(text, "and ends with loupe review") || !strings.Contains(text, confirmStray) {
		t.Errorf("the confirmation lost an end of the refusal or the keys that answer:\n%s", view)
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

// A refusal taller than every row the screen can give it keeps its opening and its last row, which carries the fix,
// and says how many rows between were left out (demo feedback: shorten the middle).
func TestRefusalTallerThanTheScreenKeepsItsEnds(t *testing.T) {
	var ids []string
	for i := 1; i <= 200; i++ {
		ids = append(ids, fmt.Sprintf("f-%03d", i))
	}
	refusal := "the draft is not ready: pending findings " + strings.Join(ids, ", ") + "; loupe review"
	for _, sc := range []struct {
		name string
		show func(*Model)
	}{
		{"list", func(m *Model) {}},
		{"detail", func(m *Model) { mustOpen(m, "f-001") }},
		{"action", func(m *Model) { m.view, m.pick = viewAction, 0 }},
	} {
		m := modelOf(t, newFixture(t), utf8Env, 60, 12)
		sc.show(m)
		m.say(style.Warn, refusal)
		view := m.View().Content
		if lines := strings.Split(view, "\n"); len(lines) != 12 {
			t.Fatalf("%s is %d rows at 60x12:\n%s", sc.name, len(lines), view)
		}
		text := paneText(view)
		if !strings.Contains(text, "the draft is not ready") || !strings.Contains(text, "f-200; loupe review") || !strings.Contains(view, "more rows") {
			t.Errorf("%s lost an end of the refusal or the mark between them:\n%s", sc.name, view)
		}
	}
}

// A pane the human types or edits in is drawn in the note color while it has the keys: send back, edit, and the
// review message while typing. The blurred message pane returns to the accent (demo feedback).
func TestInputPanesAreAmber(t *testing.T) {
	env := map[string]string{"LANG": "en_US.UTF-8"}
	amber := func(m *Model, title string) bool {
		edge := m.styles.ToneStyle(style.ToneNote)
		return strings.Contains(m.View().Content, edge.Render("╭")+edge.Render("─")+m.styles.TitleTab(title, style.ToneNote))
	}
	for _, sc := range []struct {
		name, title string
		open        func(*Model)
	}{
		{"send back", "Send back f-001", func(m *Model) { mustOpen(m, "f-001"); m.Update(runes("s")) }},
		{"edit", "Edit f-001", func(m *Model) { mustOpen(m, "f-001"); m.Update(runes("e")) }},
	} {
		m := modelOf(t, newFixture(t), env, 100, 30)
		m.styles.SetColorProfile(colorprofile.TrueColor)
		sc.open(m)
		if !amber(m, sc.title) {
			t.Errorf("the %s pane is not amber:\n%q", sc.name, m.View().Content)
		}
	}
	m := modelOf(t, newFixture(t), env, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	showConfirmation(m)
	m.confirm.typing = true
	if !amber(m, "your message") {
		t.Errorf("the message pane is not amber while typing:\n%q", m.View().Content)
	}
	m.confirm.typing = false
	if amber(m, "your message") {
		t.Errorf("the blurred message pane is still amber:\n%q", m.View().Content)
	}
}

// Only the pane taking the keys has a solid title: the findings in the list, the finding in detail until a composer
// opens, the composer then, and the review message only while typing (demo feedback).
func TestOnlyTheFocusedPaneHasATab(t *testing.T) {
	env := map[string]string{"LANG": "en_US.UTF-8"}
	m := modelOf(t, newFixture(t), env, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	// A tab is a fill, so the title on screen must carry the fill escape, not only match TitleTab (review finding).
	filled := func(tab string) string {
		if !strings.Contains(tab, "48;2;") {
			t.Fatalf("the tab %q has no fill", tab)
		}
		return tab
	}
	tab := func(text string, tone style.Tone) bool {
		return strings.Contains(m.View().Content, filled(m.styles.TitleTab(text, tone)))
	}
	if !tab("Findings", style.ToneAccent) || tab("Reviewer's summary", style.ToneAccent) {
		t.Errorf("the list's tab is not on the findings alone:\n%q", m.View().Content)
	}
	mustOpen(m, "f-001")
	if !tab("f-001", style.ToneAccent) {
		t.Errorf("the open finding's id is not a tab:\n%q", m.View().Content)
	}
	m.Update(runes("s"))
	if !tab("Send back f-001", style.ToneNote) || tab("f-001", style.ToneAccent) {
		t.Errorf("the tab did not move to the send-back composer:\n%q", m.View().Content)
	}
	c := modelOf(t, newFixture(t), env, 100, 30)
	c.styles.SetColorProfile(colorprofile.TrueColor)
	showConfirmation(c)
	c.confirm.typing = true
	if v := c.View().Content; !strings.Contains(v, filled(c.styles.TitleTab("your message", style.ToneNote))) || strings.Contains(v, c.styles.TitleTab("Publish review", style.ToneAccent)) {
		t.Errorf("typing does not put the tab on the message:\n%q", v)
	}
	c.confirm.typing = false
	if v := c.View().Content; !strings.Contains(v, filled(c.styles.TitleTab("Publish review", style.ToneAccent))) || strings.Contains(v, c.styles.TitleTab("your message", style.ToneNote)) {
		t.Errorf("a blurred message does not return the tab to the review:\n%q", v)
	}
}

// A decision that settles a finding flashes its notice as a solid tab in the decision's color, then the notice reads as
// plain colored text until it expires (demo feedback).
func TestDecisionFlashesItsNotice(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	mustOpen(m, "f-001")
	m.Update(runes("a"))
	if m.notice == "" {
		t.Fatal("accepting said nothing")
	}
	if view := m.View().Content; !strings.Contains(view, m.styles.TitleTab(m.notice, style.ToneGood)) {
		t.Errorf("the accept notice does not flash:\n%q", view)
	}
	m.Update(flashEnded{m.flashGen})
	view := m.View().Content
	if strings.Contains(view, m.styles.TitleTab(m.notice, style.ToneGood)) || !strings.Contains(view, m.styles.Good.Render(m.notice)) {
		t.Errorf("the notice did not settle to plain colored text after the flash:\n%q", view)
	}
}

// The flash only changes the notice's background: its text stays in the same cells when the flash ends (demo
// feedback).
func TestDecisionFlashKeepsTheNoticeInPlace(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	mustOpen(m, "f-001")
	m.Update(runes("a"))
	column := func() (int, int) {
		for i, row := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if c := strings.Index(row, m.notice); c >= 0 {
				return i, c
			}
		}
		return -1, -1
	}
	row, flashing := column()
	m.Update(flashEnded{m.flashGen})
	afterRow, after := column()
	if flashing < 0 || row != afterRow || flashing != after {
		t.Errorf("the notice moved from row %d column %d to row %d column %d when the flash ended", row, flashing, afterRow, after)
	}
}
