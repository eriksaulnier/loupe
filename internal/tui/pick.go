package tui

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/secondshift-labs/loupe/internal/draft"
	"github.com/secondshift-labs/loupe/internal/render"
	"github.com/secondshift-labs/loupe/internal/style"
)

// pickFor is what picking an option does to the open finding.
type pickFor int

const (
	// pickAccept accepts the finding with the picked option.
	pickAccept pickFor = iota
	// pickReinstate reinstates a withdrawn finding and accepts it with the picked option.
	pickReinstate
	// pickAcceptNote holds the picked option and opens the note composer, which accepts with both.
	pickAcceptNote
)

// pickedKey is the key of the option f's current decision picked, or "" when f is not accepted with a pick.
func (m *Model) pickedKey(f draft.Finding) string {
	if len(f.Options) == 0 || draft.Dispositions(m.draft)[f.ID] != draft.DispositionAccepted {
		return ""
	}
	return m.draft.Decisions[f.ID].Choice
}

// openPick opens the option picker under the open finding, on the pick it holds or else the recommended option. The
// finding is on screen already, but a repeated key can still reach the picker before it is drawn, so keys that pick are
// dropped for the settle window, as after a decision.
func (m *Model) openPick(kind pickFor) tea.Cmd {
	f, _ := m.openedFinding()
	m.picking, m.pickFor, m.notice = true, kind, ""
	m.pickSel = 0
	want := m.pickedKey(f)
	if want == "" {
		for _, o := range f.Options {
			if o.Recommended {
				want = o.Key
			}
		}
	}
	if i := slices.IndexFunc(f.Options, func(o draft.Option) bool { return o.Key == want }); i >= 0 {
		m.pickSel = i
	}
	if m.cfg.Screen != nil {
		return nil
	}
	m.settling = true
	return tea.Tick(settleAfterDecision, func(time.Time) tea.Msg { return settledMsg{} })
}

func (m *Model) updatePick(msg tea.KeyPressMsg) tea.Cmd {
	f, _ := m.openedFinding()
	key := msg.String()
	if key == "esc" {
		m.picking, m.pickNote = false, ""
		return nil
	}
	if m.settling || len(f.Options) == 0 {
		return nil
	}
	m.pickSel = min(m.pickSel, len(f.Options)-1)
	switch key {
	case "j", "down":
		m.pickSel = min(m.pickSel+1, len(f.Options)-1)
	case "k", "up":
		m.pickSel = max(m.pickSel-1, 0)
	case "enter":
		return m.pickOption(f.Options[m.pickSel].Key)
	default:
		if slices.ContainsFunc(f.Options, func(o draft.Option) bool { return o.Key == key }) {
			return m.pickOption(key)
		}
	}
	return nil
}

// pickOption is the one way an option is picked: enter on a row and its digit both come here, and so will a click.
// It is not named pick, which the action and inline pickers' cursor already is.
func (m *Model) pickOption(key string) tea.Cmd {
	id := m.openID
	m.picking = false
	now := m.cfg.Now()
	var closed []string
	if m.pickFor == pickAcceptNote {
		f, _ := m.openedFinding()
		m.pickHeld = key
		text := heldNote(m.draft, f, draft.DispositionAccepted)
		if m.pickNote != "" {
			text, m.pickNote = m.pickNote, ""
		}
		return m.openNote(noteAccept, text)
	}
	if m.pickFor == pickReinstate {
		return m.decideAndShow(func(d *draft.Draft) (err error) {
			closed, err = draft.Reinstate(d, id, key, m.rules, now)
			return err
		}, func() string {
			return decidedNotice(m.glyphs.Sep, m.glyphs.Accepted, id, "reinstated and accepted with "+key, closed, draft.NoteResolved)
		})
	}
	return m.decideAndShow(func(d *draft.Draft) (err error) {
		closed, err = draft.Accept(d, id, key, now)
		return err
	}, func() string {
		return decidedNotice(m.glyphs.Sep, m.glyphs.Accepted, id, "accepted with "+key, closed, draft.NoteResolved)
	})
}

// pickView is the option picker, a pane under the finding: one row per option, the selected one marked.
func (m *Model) pickView() []string {
	f, _ := m.openedFinding()
	inner, _ := style.PaneInner(m.width, 0)
	var lines []string
	for i, o := range f.Options {
		mark := ""
		if o.Recommended {
			mark = " " + m.glyphs.Sep + " recommended"
		}
		lines = append(lines, m.optionRow(i == m.pickSel, o.Key, render.ForDisplay(render.OneLine(o.Label)), mark, inner)...)
	}
	verb := "accepts"
	switch m.pickFor {
	case pickReinstate:
		verb = "reinstates"
	case pickAcceptNote:
		verb = "picks"
	}
	way := m.styles.HintTitle("enter " + verb + " " + m.glyphs.Sep + " esc cancels")
	return m.styles.Pane(style.Pane{Width: m.width, Height: len(lines) + style.PaneRows, Lines: lines,
		Titles: style.PaneTitles{TopLeft: "Pick for " + m.openID, BottomRight: way}, Tone: style.ToneNote, Focus: true})
}

// pickFooter is the picker's keys.
func (m *Model) pickFooter() []string {
	f, _ := m.openedFinding()
	verb := "accept"
	switch m.pickFor {
	case pickReinstate:
		verb = "reinstate"
	case pickAcceptNote:
		verb = "pick"
	}
	digits := "1"
	if n := len(f.Options); n > 1 {
		digits = fmt.Sprintf("1-%s", strconv.Itoa(n))
	}
	return m.keys(style.KeyGroup{{Key: "enter", Verb: verb}, {Key: digits, Verb: "pick"},
		{Key: m.arrows(m.glyphs.Up, m.glyphs.Down), Verb: "move"}, {Key: "esc", Verb: "cancel"}})
}

// optionIndent is where an option's label starts: the cursor, a space, the one-digit key and two spaces.
const optionIndent = 5

// optionRow is one picker row: the cursor, the option's key, its label and its mark. The label is cut with the ellipsis
// to leave the mark room, since the option's section above shows the label whole. The publish popup's pickerRow pads
// its name column for action names, which leaves a one-digit key far from its label.
func (m *Model) optionRow(selected bool, key, label, mark string, inner int) []string {
	cursor, keyStyle := " ", m.styles.Text()
	if selected {
		cursor, keyStyle = m.styles.Emphasis(m.styles.Cursor).Render(m.styles.PickerCursor()), m.styles.Emphasis(m.styles.Accent)
	}
	label = m.styles.TruncRight(label, inner-optionIndent-style.Width(mark))
	return []string{cursor + " " + keyStyle.Render(key) + "  " + m.styles.Text().Render(label+mark)}
}
