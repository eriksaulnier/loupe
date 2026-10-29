package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/pane"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

// noticeExpiry is how long a success notice stays in its pane's border. Refusals and errors do not expire.
var noticeExpiry = 2 * time.Second

// noticeExpired clears the success notice it was scheduled for, and nothing newer.
type noticeExpired struct{ gen int }

// say sets the notice and how it reads: Good for something recorded, Warn for a refusal or a dead end. Every notice
// takes a new generation, so a timer left from an older one cannot clear it.
func (m *Model) say(kind style.Kind, text string) {
	m.notice, m.noticeKind = text, kind
	m.noticeGen++
}

// expireNotice is the timer for a success notice set during this update, if one was.
func (m *Model) expireNotice(before int) tea.Cmd {
	if m.noticeGen == before || m.notice == "" || m.noticeKind != style.Good {
		return nil
	}
	gen := m.noticeGen
	return tea.Tick(noticeExpiry, func(time.Time) tea.Msg { return noticeExpired{gen} })
}

// clipToWindow is the frame's last guard against a line wider than the window. Tests turn it off to see whether any
// view relies on it.
var clipToWindow = true

// shell is the one layout every review screen renders through: the one-row header, the body rows and the measured
// footer. Body and footer are sanitized here; the header is built from sanitized text with its one trusted link added
// afterwards, so it is the only row that may carry OSC 8.
func (m *Model) shell(body, footer []string) string {
	for i, l := range body {
		body[i] = m.clip(render.ForDisplayANSI(l))
	}
	for i, l := range footer {
		footer[i] = m.clip(render.ForDisplayANSI(l))
	}
	return m.styles.ReviewFrame(m.clip(m.reviewHeader()), body, footer, m.width, m.height)
}

func (m *Model) clip(line string) string {
	if clipToWindow && style.Width(line) > m.width {
		return ansi.Truncate(line, m.width, "")
	}
	return line
}

// bodyTop is the first row of the body: under the header and the blank row that follows it where there is room.
func (m *Model) bodyTop() int {
	return 1 + style.ReviewLayout{Height: m.height}.Gap()
}

// bodyRows is what the shell leaves between the header and a footer of footerRows.
func (m *Model) bodyRows(footerRows int) int {
	return style.ReviewLayout{Width: m.width, Height: m.height, Footer: footerRows}.Body()
}

// reviewHeader is the header every screen opens with. The standalone confirmation has no draft, so it names the pull
// request without a count.
func (m *Model) reviewHeader() string {
	if m.draft == nil {
		return m.styles.ReviewHeader(style.ReviewHeading{Ref: render.ForDisplay(m.confirm.title.ref), NoCount: true, Reserve: m.hostReserve()}, m.width)
	}
	order, decided, total := progressOf(m.draft, m.order)
	return m.styles.ReviewHeader(style.ReviewHeading{
		Ref: render.ForDisplay(m.ref()), URL: m.target.URL, Round: m.target.Round, Title: m.titleText(),
		Progress: order, Current: m.currentCell(), Decided: decided, Total: total, Reserve: m.hostReserve(),
	}, m.width)
}

// hostReserve is the header's columns a pane host draws over, which the header leaves blank.
func (m *Model) hostReserve() int {
	return pane.HeaderReserve(m.cfg.Getenv)
}

// currentCell is the progress cell of the open finding in detail, counting from 1. It is 0 on every other screen,
// where the list's selection already shows the place, and for a withdrawn finding, which has no cell.
func (m *Model) currentCell() int {
	if m.view != viewDetail {
		return 0
	}
	id := m.openID
	dispositions := draft.Dispositions(m.draft)
	cell := 0
	for _, f := range m.order {
		if dispositions[f.ID] == draft.DispositionWithdrawn {
			continue
		}
		cell++
		if f.ID == id {
			return cell
		}
	}
	return 0
}

// progressOf projects the derived dispositions onto the bar, in list order. A withdrawn finding has no cell and is in
// neither count; a pending finding with an open note, then a blocking one, take their own colors.
func progressOf(d *draft.Draft, order []draft.Finding) (cells []style.ProgressState, decided, total int) {
	dispositions, notes := draft.Dispositions(d), openNotes(d)
	for _, f := range order {
		var st style.ProgressState
		switch dispositions[f.ID] {
		case draft.DispositionAccepted:
			st = style.ProgressAccepted
		case draft.DispositionExcluded:
			st = style.ProgressExcluded
		case draft.DispositionPending:
			_, open := notes[f.ID]
			switch {
			case open:
				st = style.ProgressNote
			case f.Blocking:
				st = style.ProgressBlocking
			default:
				st = style.ProgressPending
			}
		default:
			continue
		}
		if st == style.ProgressAccepted || st == style.ProgressExcluded {
			decided++
		}
		total++
		cells = append(cells, st)
	}
	return cells, decided, total
}

// keys lays out the footer groups for the window.
func (m *Model) keys(groups ...style.KeyGroup) []string {
	return m.styles.KeyGroups(groups, m.width)
}

// arrows names two arrow keys as one hint key: ↑↓ where the glyphs exist, up/down where they do not.
func (m *Model) arrows(a, b string) string {
	if m.glyphs.Tier == style.ASCII {
		return a + "/" + b
	}
	return a + b
}

// groupHints sorts a view's hints into the decide, move and app groups the footer draws.
func groupHints(hints []style.Hint) []style.KeyGroup {
	var decide, move, app style.KeyGroup
	for _, h := range hints {
		switch h.Role {
		case style.RoleDecision:
			decide = append(decide, h)
		case style.RoleNav, style.RoleFile:
			move = append(move, h)
		default:
			app = append(app, h)
		}
	}
	return []style.KeyGroup{decide, move, app}
}

// borderNotice is the notice drawn at the right of the active pane's top border, when it is short and not a refusal.
// room is the cells that border has for it.
func (m *Model) borderNotice(room int) string {
	if m.notice == "" || m.noticeKind == style.Warn || style.Width(m.notice) > room {
		return ""
	}
	text := render.ForDisplay(m.notice)
	// A decision's notice flashes as a solid tab first; the pane draws it with TopNoticeTab, so it keeps its cells.
	if m.flashing(room) {
		return m.styles.TitleTab(text, m.flashTone)
	}
	return m.styles.Of(m.noticeKind).Render(text)
}

// flashing is whether the border notice shows as a flashing tab: a decision's notice in its flash, in color, with
// room in the border for it.
func (m *Model) flashing(room int) bool {
	return m.styles.Color && m.notice != "" && m.flashGen != 0 && m.flashGen == m.noticeGen &&
		style.Width(render.ForDisplay(m.notice)) <= room
}

// withBorderNotice puts the notice that fits p's top border into it, and returns the room it had there.
func (m *Model) withBorderNotice(p *style.Pane) int {
	room := m.styles.NoticeRoom(*p)
	p.Titles.TopNotice, p.Titles.TopNoticeTab = m.borderNotice(room), m.flashing(room)
	return room
}

// noticeRows are a notice that does not belong in a border, wrapped whole into the pane so its corrective text is
// never clipped: every refusal, and anything too long for room.
func (m *Model) noticeRows(width, room int) []string {
	if m.notice == "" || m.borderNotice(room) != "" {
		return nil
	}
	lines := strings.Split(m.styles.Wrap(render.ForDisplay(m.notice), width, ""), "\n")
	for i, l := range lines {
		lines[i] = m.styles.Of(m.noticeKind).Render(l)
	}
	return lines
}

// withNotice puts the notice under a pane's document rows in rows of content. The document gives way, below the
// viewport's one-row floor if it must, so a refusal that fits the pane is never cut. One taller than every row keeps
// its opening rows and its last row, which carries the fix, and says how many rows between were left out.
func (m *Model) withNotice(doc, notice []string, rows int) []string {
	if len(notice) > rows {
		notice = m.shortenNotice(notice, rows)
	}
	keep := max(0, min(len(doc), rows-len(notice)))
	return append(doc[:keep:keep], notice...)
}

// shortenNotice fits a notice into rows by dropping rows from its middle, never its first or its last.
func (m *Model) shortenNotice(notice []string, rows int) []string {
	switch {
	case rows <= 0:
		return nil
	case rows == 1:
		return notice[len(notice)-1:]
	case rows == 2:
		return []string{notice[0], notice[len(notice)-1]}
	}
	head := rows - 2
	left := len(notice) - head - 1
	mark := m.styles.Overlay().Render(fmt.Sprintf("%s %d more %s", m.glyphs.Ellipsis, left, plural(left, "row")))
	return slices.Concat(notice[:head], []string{mark}, notice[len(notice)-1:])
}

// noticeRoom is the bottom-left border's room in a pane of width beside a right title, which the publish popup's
// notice and the summary's command take.
func noticeRoom(width int, right string) int {
	return width - 2 - 4 - style.Width(right) - 3 - 1
}

// helpOverlay draws help as an opaque pane over the dimmed screen it was opened from, which keeps its own state.
func (m *Model) helpOverlay(base string) string {
	lines := m.helpLines()
	width := m.popupWidth()
	inner, _ := style.PaneInner(width, 0)
	footer := m.helpFooter()
	height, visible := m.helpRows()
	top := min(m.helpTop, max(0, len(lines)-visible))
	shown := lines[top:]
	for i, l := range shown {
		shown[i] = render.ForDisplayANSI(m.styles.TruncRight(l, inner))
	}
	pane := m.styles.Pane(style.Pane{
		Width: width, Height: height, Lines: shown,
		Titles: style.PaneTitles{TopLeft: "Help", TopRight: m.styles.Text().Render(strings.ToLower(m.helpCurrent().short)), BottomRight: m.styles.Text().Render("esc closes")},
		Scroll: style.Scroll{Offset: top, Visible: visible, Total: len(lines)},
		Focus:  true,
	})
	return m.overlay(base, m.viewFooterRows(), pane, footer)
}

// overlay draws pane over the dimmed base screen, whose own footer of baseFooter rows is blanked: footer replaces it,
// and its keys are the only ones that work now. The header stays as drawn, link and colors included, so the screen
// still says whose review it is; the pane is composed over the body only.
func (m *Model) overlay(base string, baseFooter int, pane, footer []string) string {
	region := m.height - len(footer)
	baseLines := strings.Split(m.styles.Muted(base), "\n")
	for i := max(0, len(baseLines)-baseFooter); i < len(baseLines); i++ {
		baseLines[i] = ""
	}
	for len(baseLines) < region {
		baseLines = append(baseLines, "")
	}
	out := strings.Split(m.styles.Composite(strings.Join(baseLines[1:region], "\n"), pane, m.width, m.bodyTop()-1, region-1), "\n")
	for i, l := range footer {
		footer[i] = m.clip(render.ForDisplayANSI(l))
	}
	rows := append([]string{m.clip(m.reviewHeader())}, out[:min(len(out), region-1)]...)
	return strings.Join(append(rows, footer...), "\n")
}

// popupWidth is the width of a pane drawn over the screen, such as help or a publish step.
func (m *Model) popupWidth() int { return min(m.width-4, 72) }

// helpFooter is the keys help offers while it is open.
func (m *Model) helpFooter() []string {
	return m.keys(style.KeyGroup{{Key: m.arrows(m.glyphs.Up, m.glyphs.Down), Verb: "scroll"}}, style.KeyGroup{{Key: "esc/?/q", Verb: "close help"}})
}

// helpRows is help's pane height and the rows it shows its lines in, which both the drawing and the scroll read so
// the scroll stops where the pane does. The header stays in view above the pane, so help still says whose review it is.
func (m *Model) helpRows() (height, visible int) {
	height = min(len(m.helpLines())+style.PaneRows, m.height-len(m.helpFooter())-2)
	return height, max(1, height-style.PaneRows)
}

// position is where the selected or open finding sits among every row, withdrawn ones included.
func position(i, n int) string {
	if n == 0 {
		return "0 of 0"
	}
	return fmt.Sprintf("%d of %d", i+1, n)
}
