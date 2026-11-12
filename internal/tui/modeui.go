package tui

import (
	"fmt"
	"path"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/local"
	"github.com/eriksaulnier/loupe/internal/mode"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

// modeUI is what review offers on a run of one mode: its hints, its help, the keys only it binds and how the run
// ends. New picks it once, so no other code in this package asks a run's kind.
type modeUI interface {
	// word is the on-screen word for an action or a party in notices and note panes. PR runs keep their words;
	// a local run speaks of rejecting and asking, and names the applier.
	word(m *Model, w string) string
	// settleKeys are the decision keys a settling detail drops.
	settleKeys() string
	helpSections(g GlyphSet) map[view]helpSection
	listHints(m *Model) []style.Hint
	detailHints(m *Model, f draft.Finding) []style.Hint
	// listKey and detailKey act on a key only this mode binds, and report whether they did.
	listKey(m *Model, key string) (tea.Cmd, bool)
	detailKey(m *Model, f draft.Finding, key string) (tea.Cmd, bool)
	// heading is what the header names: a pull request's reference, link and round, or a local run's place. runRef is
	// the run as loupe's commands name it.
	heading(m *Model) style.ReviewHeading
	runRef(m *Model) string
	// question reports whether the list marks f as a question.
	question(f draft.Finding) bool
	// watchesPR reports whether review checks the live pull request; a local run has none and contacts no service.
	watchesPR() bool
	// opening is the view review opens on.
	opening(m *Model) view
	// nextAfter is the finding to open after a decision on the finding at index i, or -1 to return to the list.
	nextAfter(m *Model, i int) int
	// atList is what follows once a decision has returned the view to the list.
	atList(m *Model) tea.Cmd
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

func (publishUI) word(_ *Model, w string) string { return w }

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
			{leftRight, "previous / next finding", "[/], n for next"},
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

func (publishUI) opening(*Model) view { return viewList }

func (publishUI) nextAfter(m *Model, i int) int {
	if i < 0 || i+1 >= len(m.order) {
		return -1
	}
	return i + 1
}

func (publishUI) atList(*Model) tea.Cmd { return nil }

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

func (applyUI) word(m *Model, w string) string {
	switch w {
	case "excluded":
		return "rejected"
	case "Send back":
		return "Ask about"
	case "sent back":
		return "asked"
	case "the agent":
		return m.applier()
	}
	return w
}

func (applyUI) settleKeys() string { return "atxcsurd" }

func (applyUI) helpSections(g GlyphSet) map[view]helpSection {
	upDown, leftRight := g.Up+"/"+g.Down, g.Left+"/"+g.Right
	return map[view]helpSection{
		viewList: {"Finding list", "list", []helpKey{
			{upDown, "move", "k/j"},
			{"enter", "open the finding", ""},
			{"tab", "expand or collapse the summary", ""},
			{"p", "return the decisions to the agent, with a closing note if you like", ""},
			{"ctrl+r", "reload the draft from disk", ""},
		}},
		viewDetail: {"Finding detail", "detail", []helpKey{
			{leftRight, "previous / next finding", "[/], n for next"},
			{upDown, "scroll the finding", "k/j"},
			{"PgUp/PgDn", "scroll one page", ""},
			{"space", "scroll one page down", ""},
			{"a", "accept; resolves its open notes", ""},
			{"t", "tweak: accept with a note; enter accepts, esc cancels", ""},
			{"x", "reject; dismisses its open notes", ""},
			{"c", "reject with a reason; enter rejects, esc cancels", ""},
			{"s", "ask: send back with a question; enter sends, esc cancels", ""},
			{"u", "restore or reinstate the finding", ""},
			{"r / d", "resolve / dismiss its open note", ""},
			{"f", "whole-file diff", ""},
			{"ctrl+r", "reload the draft from disk", ""},
			{"l / esc", "back to the list", ""},
		}},
		viewFileDiff: fileDiffHelp(g),
	}
}

func (applyUI) listHints(m *Model) []style.Hint {
	return []style.Hint{
		{Key: m.arrows(m.glyphs.Up, m.glyphs.Down), Verb: "move", Role: style.RoleNav},
		{Key: "enter", Verb: "open", Role: style.RoleNav},
		{Key: "tab", Verb: "summary", Role: style.RoleNav},
		m.endHint("return to " + m.applier()),
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
	// On a finding with options, a and t open the picker, so they say pick.
	accept, tweak := "accept", "tweak"
	if len(f.Options) > 0 {
		accept, tweak = "pick", "pick + note"
	}
	switch draft.Dispositions(m.draft)[f.ID] {
	case draft.DispositionPending:
		hints = append(hints, decide("a", accept), decide("t", tweak), decide("x", "reject"), decide("c", "reject with reason"), decide("s", "ask"))
	case draft.DispositionAccepted:
		if len(f.Options) > 0 {
			hints = append(hints, decide("a", "pick again"))
		}
		hints = append(hints, decide("t", tweak), decide("x", "reject"), decide("c", "reject with reason"), decide("s", "ask"))
	case draft.DispositionExcluded:
		hints = append(hints, decide("u", "restore"), decide("c", "reason"))
	case draft.DispositionWithdrawn:
		hints = append(hints, decide("u", "reinstate"))
	}
	if hasOpenNote {
		hints = append(hints, decide("r", "resolve"), decide("d", "dismiss"))
	}
	if f.Location != nil {
		hints = append(hints, style.Hint{Key: "f", Verb: "file", Role: style.RoleFile})
	}
	return append(hints, style.Hint{Key: "l", Verb: "list", Role: style.RoleNav}, style.Hint{Key: "?", Verb: "help", Role: style.RoleHelp})
}

func (applyUI) listKey(m *Model, key string) (tea.Cmd, bool) {
	if key == "p" {
		return m.askReturn(), true
	}
	return nil, false
}

func (applyUI) detailKey(m *Model, f draft.Finding, key string) (tea.Cmd, bool) {
	switch key {
	case "t", "c":
		// Said before the input opens, since the draft would refuse the decision only after the words were typed.
		if draft.Dispositions(m.draft)[f.ID] == draft.DispositionWithdrawn {
			m.say(style.Warn, fmt.Sprintf("%s is withdrawn; u reinstates it", f.ID))
			return nil, true
		}
		if key == "c" {
			return m.openNote(noteExclude, heldNote(m.draft, f, draft.DispositionExcluded)), true
		}
		if len(f.Options) > 0 {
			return m.openPick(pickAcceptNote), true
		}
		return m.openNote(noteAccept, heldNote(m.draft, f, draft.DispositionAccepted)), true
	case "w":
		// Absent here, and swallowed so the shared handler cannot give it a pull request run's meaning.
		return nil, true
	case "l":
		return m.backToList(), true
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

// heading carries the badge that says the decisions go to the human's own agent and nowhere else.
func (applyUI) heading(m *Model) style.ReviewHeading {
	full, mid := "local "+m.glyphs.Sep+" returns to "+m.applier(), "local "+m.glyphs.Sep+" "+m.applier()
	if m.glyphs.Tier == style.ASCII {
		full = "local -> " + m.applier()
		mid = full
	}
	return style.ReviewHeading{Local: localPlace(m.target), Badges: []string{full, mid, "local"}}
}

func (applyUI) runRef(m *Model) string { return localRef(m.target) }

func (applyUI) question(f draft.Finding) bool { return f.Proposal == "" && len(f.Options) == 0 }

func (applyUI) watchesPR() bool { return false }

// opening is the start screen while a finding waits, so the human starts the cards with one key.
func (applyUI) opening(m *Model) view {
	if len(draft.ReadinessOf(m.draft).Pending) > 0 {
		return viewStart
	}
	return viewList
}

// nextAfter is the next pending finding, wrapping past the end, so the cards skip what is already decided.
func (applyUI) nextAfter(m *Model, i int) int {
	dispositions := draft.Dispositions(m.draft)
	for step := 1; step < len(m.order); step++ {
		j := (i + step) % len(m.order)
		if dispositions[m.order[j].ID] == draft.DispositionPending {
			return j
		}
	}
	return -1
}

// atList opens the return step once nothing is left to decide.
func (applyUI) atList(m *Model) tea.Cmd {
	if !draft.ReadinessOf(m.draft).Ready {
		return nil
	}
	return m.openReturn()
}

// plainCode is true on a --files run, whose diff adds each file whole only because it was captured from nothing.
func (applyUI) plainCode(t run.Target) bool { return t.Local != nil && t.Local.Capture == "files" }

// localPlace is where a local run's work lives, for its header: the repository's name, the branch and what capture
// took.
func localPlace(t run.Target) *style.LocalPlace {
	p := &style.LocalPlace{}
	l := t.Local
	if l == nil {
		return p
	}
	p.Repo = path.Base(l.Prefix)
	if l.Prefix == "" {
		p.Repo = filepath.Base(l.Root)
	}
	p.Repo, p.Branch = render.ForDisplay(p.Repo), render.ForDisplay(l.Branch)
	switch l.Capture {
	case local.FormFiles:
		p.Capture = fmt.Sprintf("%d files", len(l.Files))
		if len(l.Files) == 1 {
			p.Capture = "1 file"
		}
	case local.FormChanged:
		p.Capture = "uncommitted changes"
		if l.BaseRef != "" {
			p.Capture = "changes since " + render.ForDisplay(l.BaseRef)
		}
	case local.FormNoFiles:
		p.Capture = "no files"
	}
	return p
}

func localRef(t run.Target) string {
	ref := run.Ref{Local: 1}
	if t.Local != nil {
		ref = run.Ref{Local: t.Local.N, Prefix: t.Local.Prefix}
	}
	return ref.String()
}
