package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/style"
)

type rewriteField int

const (
	rewriteFocusTitle rewriteField = iota
	rewriteFocusBody
)

// keptRewrite is text the human left with esc, and the revision it was typed against, so reopening it on a finding
// the agent has since changed can say so. Typed holds only the fields the human changed: a field left alone takes
// the finding's text on reopening, so the agent's newer version is not saved back as the human's.
type keptRewrite struct {
	Typed [2]*string
	Rev   int
}

// openedText is a field's text in the finding the editor stands on, and as the input shows it. The input turns a
// tab into spaces and a CRLF into one line break, so while the human leaves a field alone, the finding's text is
// what it holds.
type openedText struct{ Source, Shown string }

// setRewriteValue puts s in input with each CRLF as one line break: the input reads a lone CR as a break of its own,
// so a CRLF left in would come back as a paragraph break in any field the human types in.
func setRewriteValue(input *textarea.Model, s string) {
	input.SetValue(strings.ReplaceAll(s, "\r\n", "\n"))
}

// newRewriteInput is a textarea that grows with its text up to the height sizeRewrite allows, then scrolls to its
// cursor. MaxContentHeight is set so MaxHeight bounds only the view: left at zero, MaxHeight would also stop input.
func newRewriteInput(newlines bool) textarea.Model {
	input := textarea.New()
	input.ShowLineNumbers, input.DynamicHeight, input.MinHeight, input.MaxContentHeight = false, true, 1, 1<<30
	input.SetStyles(textarea.Styles{})
	input.SetVirtualCursor(false)
	input.SetPromptFunc(0, func(textarea.PromptInfo) string { return "" })
	input.KeyMap.InsertNewline.SetEnabled(newlines)
	return input
}

// openRewrite opens the editor on f as it stands, with each field kept for f in place of its own.
func (m *Model) openRewrite(f draft.Finding) tea.Cmd {
	if err := draft.Rewritable(f); err != nil {
		m.say(style.Warn, refusalNotice(err))
		return nil
	}
	m.rewriting, m.notice = true, ""
	m.rewriteRev = f.Rev
	setRewriteValue(&m.rewriteTitle, f.Title)
	setRewriteValue(&m.rewriteBody, f.Body)
	m.rewriteOpened = [2]openedText{{f.Title, m.rewriteTitle.Value()}, {f.Body, m.rewriteBody.Value()}}
	if kept, ok := m.keptRewrites[f.ID]; ok {
		m.rewriteRev = kept.Rev
		if kept.Rev != f.Rev {
			m.say(style.Note, fmt.Sprintf("%s changed since the rewrite began; your text is kept", f.ID))
		}
		for i, input := range m.rewriteInputs() {
			if typed := kept.Typed[i]; typed != nil {
				input.SetValue(*typed)
			}
		}
	}
	m.sizeRewrite()
	return m.focusRewrite(rewriteFocusTitle)
}

func (m *Model) rewriteInputs() [2]*textarea.Model {
	return [2]*textarea.Model{&m.rewriteTitle, &m.rewriteBody}
}

// rewriteTyped is the editor's text in each field the human changed, and nil in each they left alone.
func (m *Model) rewriteTyped() (typed [2]*string) {
	for i, input := range m.rewriteInputs() {
		if value := input.Value(); value != m.rewriteOpened[i].Shown {
			typed[i] = &value
		}
	}
	return typed
}

// rewriteText is the editor's title and body, with each field the human left alone as the finding has it.
func (m *Model) rewriteText() (title, body string) {
	text := [2]string{m.rewriteOpened[rewriteFocusTitle].Source, m.rewriteOpened[rewriteFocusBody].Source}
	for i, typed := range m.rewriteTyped() {
		if typed != nil {
			text[i] = *typed
		}
	}
	return text[rewriteFocusTitle], text[rewriteFocusBody]
}

// rebaseRewrite stands the editor on f as the agent left it: each field the human left alone shows f's text, and
// each they changed keeps their text and cursor. The text the human now has began at f's revision.
func (m *Model) rebaseRewrite(f draft.Finding) {
	m.rewriteRev = f.Rev
	source := [2]string{f.Title, f.Body}
	for i, input := range m.rewriteInputs() {
		if input.Value() == m.rewriteOpened[i].Shown && source[i] != m.rewriteOpened[i].Source {
			setRewriteValue(input, source[i])
		}
		shown := newRewriteInput(i == int(rewriteFocusBody))
		setRewriteValue(&shown, source[i])
		m.rewriteOpened[i] = openedText{source[i], shown.Value()}
	}
	m.sizeRewrite()
}

func (m *Model) focusRewrite(field rewriteField) tea.Cmd {
	m.rewriteFocus = field
	if field == rewriteFocusTitle {
		m.rewriteBody.Blur()
		return m.rewriteTitle.Focus()
	}
	m.rewriteTitle.Blur()
	return m.rewriteBody.Focus()
}

func (m *Model) closeRewrite() {
	m.rewriting = false
	m.rewriteTitle.Blur()
	m.rewriteBody.Blur()
}

// updateRewrite drives the editor. Every key it does not name goes into the input with the keys, so q, ? and the
// arrows type or move the cursor rather than leave the finding.
func (m *Model) updateRewrite(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.keepRewrite()
		m.closeRewrite()
		return nil
	case "ctrl+s":
		return m.saveRewrite()
	case "tab", "shift+tab":
		return m.focusRewrite(1 - m.rewriteFocus)
	case "enter":
		if m.rewriteFocus == rewriteFocusTitle {
			return m.focusRewrite(rewriteFocusBody)
		}
	}
	return m.typeRewrite(msg)
}

// typeRewrite routes a keystroke or a paste into the input with the keys. A line break typed or pasted into the title
// becomes a space, since a title is one line.
func (m *Model) typeRewrite(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if m.rewriteFocus == rewriteFocusTitle {
		msg, _ = normalizeInput(msg, " ")
		m.rewriteTitle, cmd = m.rewriteTitle.Update(msg)
	} else {
		msg, _ = normalizeInput(msg, "\n")
		m.rewriteBody, cmd = m.rewriteBody.Update(msg)
	}
	m.sizeRewrite()
	return cmd
}

// keepRewrite holds the fields the human changed for their finding until the program exits, and forgets text left
// as the finding already reads.
func (m *Model) keepRewrite() {
	f, _ := m.openedFinding()
	title, body := m.rewriteText()
	if title == f.Title && body == f.Body {
		delete(m.keptRewrites, f.ID)
		return
	}
	m.keptRewrites[f.ID] = keptRewrite{Typed: m.rewriteTyped(), Rev: m.rewriteRev}
}

// saveRewrite records the text and keeps the finding's decision. A refusal, including a finding the agent changed
// since it was displayed, leaves the editor open with the text, so nothing the human typed is lost.
func (m *Model) saveRewrite() tea.Cmd {
	id := m.openID
	title, body := m.rewriteText()
	changed, disposition := false, ""
	recorded, err := m.Decide(id, func(d *draft.Draft) error {
		_, err := draft.Rewrite(d, id, title, body, m.diff, m.cfg.Now())
		if err == nil {
			changed, disposition = true, draft.Dispositions(d)[id]
		}
		return err
	})
	if err != nil {
		return m.fail(err)
	}
	if !recorded {
		if rest, stale := strings.CutPrefix(m.notice, staleNotice); stale {
			m.notice = fmt.Sprintf("the agent changed %s since it was displayed; nothing was saved and your text is kept", id) + rest
		}
		err := m.refreshDetail()
		if f, i := m.openedFinding(); i >= 0 {
			m.rebaseRewrite(f)
		}
		return m.fail(err)
	}
	m.closeRewrite()
	delete(m.keptRewrites, id)
	notice := ""
	if changed {
		notice = fmt.Sprintf("%s %s rewritten %s still %s", m.glyphs.Note, id, m.glyphs.Sep, disposition)
	}
	m.sayDecided(notice)
	return m.fail(m.refreshDetail())
}

// sizeRewrite fits both inputs in the pane: together with the divider, at most a third of the window, as the note
// is, with the title at most two rows and the body at least one.
func (m *Model) sizeRewrite() {
	width, _ := style.PaneInner(m.width, 0)
	limit := max(3, m.height/3)
	m.rewriteTitle.MaxHeight = max(1, min(2, limit-2))
	m.rewriteTitle.SetWidth(max(1, width))
	m.rewriteBody.MaxHeight = max(1, limit-1-m.rewriteTitle.Height())
	m.rewriteBody.SetWidth(max(1, width))
}

// rewriteView is the editor in its own pane under the finding: the title, a divider, the body. The top right names
// the input with the keys, so focus reads without color.
func (m *Model) rewriteView() []string {
	width, _ := style.PaneInner(m.width, 0)
	rows := strings.Split(m.styles.Render(m.rewriteTitle.View()), "\n")
	markCaret(rows, m.rewriteTitle.Cursor(), 0)
	rows = append(rows, m.styles.Dim.Render(strings.Repeat(m.glyphs.HRule, max(1, width))))
	body := strings.Split(m.styles.Render(m.rewriteBody.View()), "\n")
	markCaret(body, m.rewriteBody.Cursor(), 0)
	rows = append(rows, body...)
	field := "title"
	if m.rewriteFocus == rewriteFocusBody {
		field = "body"
	}
	return m.styles.Pane(style.Pane{Width: m.width, Height: len(rows) + style.PaneRows, Lines: rows,
		Titles: style.PaneTitles{TopLeft: "Rewrite " + m.openID, TopRight: m.styles.Note.Render(field)}, Tone: style.ToneNote, Focus: true})
}
