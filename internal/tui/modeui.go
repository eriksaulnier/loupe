package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/mode"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

// modeUI is what review offers on a run of one mode: its hints, its help, the keys only it binds and how the run
// ends. New picks it once, so no other code in this package asks a run's kind.
type modeUI interface {
	// settleKeys are the decision keys a settling detail drops.
	settleKeys() string
	helpSections(g GlyphSet) map[view]helpSection
	listHints(m *Model) []style.Hint
	detailHints(m *Model, f draft.Finding) []style.Hint
	// listKey and detailKey act on a key only this mode binds, and report whether they did.
	listKey(m *Model, key string) (tea.Cmd, bool)
	detailKey(m *Model, f draft.Finding, key string) (tea.Cmd, bool)
	// heading is the header's reference, link and round; runRef is the run as loupe's commands name it.
	heading(m *Model) style.ReviewHeading
	runRef(m *Model) string
	// question reports whether the list marks f as a question.
	question(f draft.Finding) bool
	// watchesPR reports whether review checks the live pull request; a local run has none and contacts no service.
	watchesPR() bool
	// plainCode reports whether the code pane and the file diff draw t's files as plain text, with no diff gutter.
	plainCode(t run.Target) bool
}

func uiFor(t run.Target) modeUI {
	if mode.Of(t).Kind() == run.KindLocal {
		return applyUI{}
	}
	return publishUI{}
}

// publishUI is a pull request run, which ends in one GitHub review.
type publishUI struct{}

func (publishUI) settleKeys() string { return "axsurdew" }

func (publishUI) helpSections(g GlyphSet) map[view]helpSection {
	upDown, leftRight := g.Up+"/"+g.Down, g.Left+"/"+g.Right
	return map[view]helpSection{
		viewList: {"Finding list", "list", []helpKey{
			{upDown, "move", "k/j"},
			{"enter", "open the finding", ""},
			{"tab", "expand or collapse the summary", ""},
			{"o", "open the pull request in the browser", ""},
			{"p", "publish once the review is ready", ""},
			{"ctrl+r", "reload the draft from disk", ""},
		}},
		viewDetail: {"Finding detail", "detail", []helpKey{
			{leftRight, "previous / next finding", "N/n"},
			{upDown, "scroll the finding", "k/j"},
			{"PgUp/PgDn", "scroll one page", ""},
			{"space", "scroll one page down", ""},
			{"a", "accept; resolves its open notes", ""},
			{"x", "exclude; dismisses its open notes", ""},
			{"s", "send back; enter sends, esc cancels", ""},
			{"e", "edit label and blocking", ""},
			{"w", "rewrite title and body; ctrl+s saves, esc keeps the text", ""},
			{"u", "restore or reinstate the finding", ""},
			{"r / d", "resolve / dismiss its open note", ""},
			{"f", "whole-file diff", ""},
			{"o", "open the pull request in the browser", ""},
			{"ctrl+r", "reload the draft from disk", ""},
			{"esc", "back to the list", ""},
		}},
		viewAction: {"Publish steps", "publish", []helpKey{
			{upDown, "move", "k/j"},
			{"enter", "choose and go on", ""},
			{"esc", "back one step", ""},
		}},
		viewFileDiff: fileDiffHelp(g),
	}
}

func fileDiffHelp(g GlyphSet) helpSection {
	return helpSection{"File diff", "file diff", []helpKey{
		{g.Up + "/" + g.Down, "move one line", "k/j"},
		{g.Left + "/" + g.Right, "previous / next marker", "[/]"},
		{"enter", "open the finding on this line", ""},
		{"esc", "back to the finding", ""},
	}}
}

func (publishUI) listHints(m *Model) []style.Hint {
	return []style.Hint{
		{Key: m.arrows(m.glyphs.Up, m.glyphs.Down), Verb: "move", Role: style.RoleNav},
		{Key: "enter", Verb: "open", Role: style.RoleNav},
		{Key: "tab", Verb: "summary", Role: style.RoleNav},
		m.openHint(),
		m.endHint("publish"),
		{Key: "?", Verb: "help", Role: style.RoleHelp},
		{Key: "q", Verb: "quit", Role: style.RoleHelp},
	}
}

func (publishUI) detailHints(m *Model, f draft.Finding) []style.Hint {
	_, hasOpenNote := firstOpenNote(m.draft, f.ID)
	hints := detailActions(m.glyphs, f, draft.Dispositions(m.draft)[f.ID], hasOpenNote)
	help := hints[len(hints)-1]
	return append(hints[:len(hints)-1], style.Hint{Key: "esc", Verb: "list", Role: style.RoleNav}, m.openHint(), help)
}

func (publishUI) listKey(m *Model, key string) (tea.Cmd, bool) {
	switch key {
	case "o":
		return m.openPR(), true
	case "p":
		ready, err := m.readyToEnd()
		if ready {
			m.view, m.pick = viewAction, 0
		}
		return m.fail(err), true
	}
	return nil, false
}

func (publishUI) detailKey(m *Model, f draft.Finding, key string) (tea.Cmd, bool) {
	switch key {
	case "e":
		m.openEdit(f)
		return nil, true
	case "o":
		return m.openPR(), true
	}
	return nil, false
}

func (publishUI) heading(m *Model) style.ReviewHeading {
	return style.ReviewHeading{Ref: m.ref(), URL: m.target.URL, Round: m.target.Round}
}

func (publishUI) runRef(m *Model) string { return fmt.Sprintf("%s@%d", m.ref(), m.target.Round) }

func (publishUI) question(draft.Finding) bool { return false }

func (publishUI) watchesPR() bool { return true }

func (publishUI) plainCode(run.Target) bool { return false }

// detailActions is the detail footer: navigation, the decisions that apply to the finding as it stands, and help. An
// action the finding cannot take is not advertised, though its key still answers with a notice.
func detailActions(g GlyphSet, f draft.Finding, disposition string, hasOpenNote bool) []style.Hint {
	pair := func(a, b string) string {
		if g.Tier == style.ASCII {
			return a + "/" + b
		}
		return a + b
	}
	hints := []style.Hint{
		{Key: pair(g.Left, g.Right), Verb: "finding", Role: style.RoleNav},
		{Key: pair(g.Up, g.Down), Verb: "scroll", Role: style.RoleNav},
	}
	decide := func(key, verb string) style.Hint {
		return style.Hint{Key: key, Verb: verb, Role: style.RoleDecision}
	}
	switch disposition {
	case draft.DispositionPending:
		hints = append(hints, decide("a", "accept"), decide("x", "exclude"), decide("s", "send back"))
	case draft.DispositionAccepted:
		hints = append(hints, decide("x", "exclude"), decide("s", "send back"))
	case draft.DispositionExcluded:
		hints = append(hints, decide("u", "restore"))
	case draft.DispositionWithdrawn:
		hints = append(hints, decide("u", "reinstate"))
	}
	if f.Included {
		hints = append(hints, decide("e", "edit"), decide("w", "rewrite"))
	}
	if hasOpenNote {
		hints = append(hints, decide("r", "resolve"), decide("d", "dismiss"))
	}
	if f.Location != nil {
		hints = append(hints, style.Hint{Key: "f", Verb: "file", Role: style.RoleFile})
	}
	return append(hints, style.Hint{Key: "?", Verb: "help", Role: style.RoleHelp})
}

// applyUI is a local run, which ends when the human returns the decisions to the agent that applies them.
type applyUI struct{}

func (applyUI) settleKeys() string { return "aAxXsurd" }

func (applyUI) helpSections(g GlyphSet) map[view]helpSection {
	upDown, leftRight := g.Up+"/"+g.Down, g.Left+"/"+g.Right
	return map[view]helpSection{
		viewList: {"Finding list", "list", []helpKey{
			{upDown, "move", "k/j"},
			{"enter", "open the finding", ""},
			{"tab", "expand or collapse the summary", ""},
			{"p", "return the decisions once every finding is decided", ""},
			{"ctrl+r", "reload the draft from disk", ""},
		}},
		viewDetail: {"Finding detail", "detail", []helpKey{
			{leftRight, "previous / next finding", "N/n"},
			{upDown, "scroll the finding", "k/j"},
			{"PgUp/PgDn", "scroll one page", ""},
			{"space", "scroll one page down", ""},
			{"a", "accept; resolves its open notes", ""},
			{"A", "accept with a note; enter accepts, esc cancels", ""},
			{"x", "exclude; dismisses its open notes", ""},
			{"X", "exclude with a reason; enter excludes, esc cancels", ""},
			{"s", "send back; enter sends, esc cancels", ""},
			{"u", "restore or reinstate the finding", ""},
			{"r / d", "resolve / dismiss its open note", ""},
			{"f", "whole-file diff", ""},
			{"ctrl+r", "reload the draft from disk", ""},
			{"esc", "back to the list", ""},
		}},
		viewFileDiff: fileDiffHelp(g),
	}
}

func (applyUI) listHints(m *Model) []style.Hint {
	return []style.Hint{
		{Key: m.arrows(m.glyphs.Up, m.glyphs.Down), Verb: "move", Role: style.RoleNav},
		{Key: "enter", Verb: "open", Role: style.RoleNav},
		{Key: "tab", Verb: "summary", Role: style.RoleNav},
		m.endHint("return"),
		{Key: "?", Verb: "help", Role: style.RoleHelp},
		{Key: "q", Verb: "quit", Role: style.RoleHelp},
	}
}

func (applyUI) detailHints(m *Model, f draft.Finding) []style.Hint {
	_, hasOpenNote := firstOpenNote(m.draft, f.ID)
	g := m.glyphs
	hints := []style.Hint{
		{Key: m.arrows(g.Left, g.Right), Verb: "finding", Role: style.RoleNav},
		{Key: m.arrows(g.Up, g.Down), Verb: "scroll", Role: style.RoleNav},
	}
	decide := func(key, verb string) style.Hint {
		return style.Hint{Key: key, Verb: verb, Role: style.RoleDecision}
	}
	switch draft.Dispositions(m.draft)[f.ID] {
	case draft.DispositionPending:
		hints = append(hints, decide("a", "accept"), decide("A", "accept with note"), decide("x", "exclude"), decide("X", "exclude with reason"),
			decide("s", "send back"))
	case draft.DispositionAccepted:
		hints = append(hints, decide("A", "accept with note"), decide("x", "exclude"), decide("X", "exclude with reason"), decide("s", "send back"))
	case draft.DispositionExcluded:
		hints = append(hints, decide("u", "restore"))
	case draft.DispositionWithdrawn:
		hints = append(hints, decide("u", "reinstate"))
	}
	if hasOpenNote {
		hints = append(hints, decide("r", "resolve"), decide("d", "dismiss"))
	}
	if f.Location != nil {
		hints = append(hints, style.Hint{Key: "f", Verb: "file", Role: style.RoleFile})
	}
	return append(hints, style.Hint{Key: "esc", Verb: "list", Role: style.RoleNav}, style.Hint{Key: "?", Verb: "help", Role: style.RoleHelp})
}

func (applyUI) listKey(m *Model, key string) (tea.Cmd, bool) {
	if key == "p" {
		return m.askReturn(), true
	}
	return nil, false
}

func (applyUI) detailKey(m *Model, f draft.Finding, key string) (tea.Cmd, bool) {
	switch key {
	case "A":
		return m.openNote(noteAccept, heldNote(m.draft, f, draft.DispositionAccepted)), true
	case "X":
		return m.openNote(noteExclude, heldNote(m.draft, f, draft.DispositionExcluded)), true
	case "t", "w":
		// Absent here, and swallowed so the shared handler cannot give them a pull request run's meaning.
		return nil, true
	}
	return nil, false
}

// heldNote is the note f's current decision holds when that decision is want, so reopening the input to change it
// starts from what the human wrote instead of an empty line that enter would save over it.
func heldNote(d *draft.Draft, f draft.Finding, want string) string {
	if draft.Dispositions(d)[f.ID] != want {
		return ""
	}
	return d.Decisions[f.ID].Note
}

func (applyUI) heading(m *Model) style.ReviewHeading {
	return style.ReviewHeading{Ref: localRef(m.target)}
}

func (applyUI) runRef(m *Model) string { return localRef(m.target) }

func (applyUI) question(f draft.Finding) bool { return f.Proposal == "" }

func (applyUI) watchesPR() bool { return false }

// plainCode is true on a --files run, whose diff adds each file whole only because it was captured from nothing.
func (applyUI) plainCode(t run.Target) bool { return t.Local != nil && t.Local.Capture == "files" }

func localRef(t run.Target) string {
	ref := run.Ref{Local: 1}
	if t.Local != nil {
		ref = run.Ref{Local: t.Local.N, Prefix: t.Local.Prefix}
	}
	return ref.String()
}
