package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

const staleNotice = "finding changed since it was displayed; nothing was recorded"

const hunkContext = 3

type view int

const (
	viewList view = iota
	viewDetail
	viewFileDiff
	viewAction
	viewInline
	viewConfirm
	viewPublishing
)

type Config struct {
	Dir    string
	Getenv func(string) string
	Now    func() time.Time
	// Output is where the program draws; it decides which colors the terminal supports.
	Output io.Writer
	// GitHub is called only when the human publishes, so reviewing never needs credentials.
	GitHub func() (github.Client, error)
}

// Width tiers: the label column goes first, then the location shrinks to a filename.
const (
	wideWidth = 100
	midWidth  = 80
)

// Model is the review program. It holds no lock: every decision goes through draft.Mutate at the displayed version.
type Model struct {
	cfg     Config
	target  run.Target
	draft   *draft.Draft
	diff    *diff.Diff
	version int
	glyphs  GlyphSet
	styles  style.Style
	view    view
	help    bool
	width   int
	height  int
	notice  string
	// noticeKind colors the notice: a recorded decision reads as success, a refusal as a warning.
	noticeKind style.Kind
	err        error

	cursor           int
	summaryCollapsed bool

	openID string
	body   viewport.Model
	hunk   []string
	// hunkTop is the first hunk row shown when the hunk is taller than its region.
	hunkTop   int
	noting    bool
	note      textinput.Model
	glamour   *glamour.TermRenderer
	wrapWidth int
	// darkBackground is asked once before the program starts, while no one else is reading the terminal's replies.
	darkBackground bool

	fileLines  []diff.ViewLine
	fileCursor int
	file       viewport.Model

	// pick is the cursor in the action and inline pickers.
	pick    int
	action  string
	confirm confirmation
	session *publishSession
	// sending is true from y until the outcome arrives; no key, not even ctrl+c, is acted on meanwhile.
	sending bool
}

func loadRun(dir string) (run.Target, *diff.Diff, *draft.Draft, error) {
	target, err := run.LoadTarget(dir)
	if err != nil {
		return run.Target{}, nil, nil, err
	}
	parsed, err := run.LoadDiff(dir, target)
	if err != nil {
		return run.Target{}, nil, nil, err
	}
	d, err := draft.Load(dir)
	if err != nil {
		return run.Target{}, nil, nil, err
	}
	return target, parsed, d, nil
}

func New(cfg Config) (*Model, error) {
	target, parsed, d, err := loadRun(cfg.Dir)
	if err != nil {
		return nil, err
	}
	st := style.New(cfg.Output, cfg.Getenv)
	note := textinput.New()
	note.Prompt = "note: "
	m := &Model{
		cfg:     cfg,
		target:  target,
		draft:   d,
		diff:    parsed,
		version: d.Version,
		glyphs:  st.Glyphs,
		styles:  st,
		width:   80,
		height:  24,
		note:    note,
		body:    viewport.New(80, 10),
		file:    viewport.New(80, 10),
		// The summary starts collapsed so the findings, not the prose, fill the first screen.
		summaryCollapsed: true,
	}
	if st.Color {
		m.darkBackground = st.R.HasDarkBackground()
	}
	return m, nil
}

// Err is the failure that ended the program, if any; refusals during review are notices, not errors.
func (m *Model) Err() error { return m.err }

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, m.fail(m.layout())
	case previewMsg:
		m.confirm, m.view = newConfirmation(msg.preview), viewConfirm
		return m, nil
	case publishDone:
		return m, m.publishFinished(msg)
	case tea.KeyMsg:
		// The confirmation comes first so that ctrl+c, like every key but y and the toggles, declines.
		if m.view == viewConfirm {
			return m, m.updateConfirm(msg)
		}
		if m.sending {
			return m, nil
		}
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.view == viewPublishing {
			return m, nil
		}
		if m.noting {
			return m, m.updateNote(msg)
		}
		if m.help {
			switch msg.String() {
			case "?", "esc", "q":
				m.help = false
			}
			return m, nil
		}
		if msg.String() == "?" {
			m.help = true
			return m, nil
		}
		switch m.view {
		case viewList:
			return m, m.updateList(msg)
		case viewDetail:
			return m, m.updateDetail(msg)
		case viewFileDiff:
			return m, m.updateFileDiff(msg)
		case viewAction:
			return m, m.updateAction(msg)
		case viewInline:
			return m, m.updateInline(msg)
		}
	}
	return m, nil
}

func (m *Model) View() string {
	if m.help {
		return m.helpView()
	}
	switch m.view {
	case viewDetail:
		return m.detailView()
	case viewFileDiff:
		return m.fileDiffView()
	case viewAction:
		return m.actionView()
	case viewInline:
		return m.inlineView()
	case viewConfirm:
		return m.confirmView(&m.confirm)
	case viewPublishing:
		keys := "ctrl+c quit"
		if m.sending {
			keys = "keys are ignored until the outcome is recorded"
		}
		return m.frame([]string{m.styles.Bold.Render(m.header())}, "", keys)
	}
	return m.listView()
}

// fail ends the program on an error that is not a refusal the human can act on.
func (m *Model) fail(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	m.err = err
	return tea.Quit
}

func (m *Model) layout() error {
	switch m.view {
	case viewDetail:
		return m.refreshDetail()
	case viewFileDiff:
		m.refreshFileDiff()
	}
	return nil
}

func (m *Model) reload() error {
	d, err := draft.Load(m.cfg.Dir)
	if err != nil {
		return err
	}
	m.draft, m.version = d, d.Version
	return nil
}

// Decide applies fn at the displayed version and reports whether it was recorded. A refusal reloads the draft and
// becomes the notice, so the human sees the finding as it now is.
func (m *Model) Decide(fn func(*draft.Draft) error) (bool, error) {
	d, notice, err := decide(m.cfg.Dir, m.version, m.cfg.Getenv, fn)
	if err != nil {
		return false, err
	}
	m.draft, m.version = d, d.Version
	m.say(style.Warn, notice)
	return notice == "", nil
}

// decide is shared by both modes. It returns the draft to display next and, when nothing was recorded, the notice.
func decide(dir string, displayed int, getenv func(string) string, fn func(*draft.Draft) error) (*draft.Draft, string, error) {
	d, err := draft.Mutate(dir, "review", &displayed, getenv, fn)
	if err == nil {
		return d, "", nil
	}
	r, ok := refusal.As(err)
	if !ok {
		return nil, "", err
	}
	reloaded, loadErr := draft.Load(dir)
	if loadErr != nil {
		return nil, "", loadErr
	}
	if r.Code == refusal.Version {
		return reloaded, staleNotice, nil
	}
	return reloaded, refusalNotice(r), nil
}

// frame is the one layout every view renders through: header lines, a body clipped or padded to the rows left, the
// notice and the key line, each clipped to the window width.
func (m *Model) frame(header []string, body, keys string) string {
	bodyHeight := max(0, m.height-len(header)-2)
	bodyLines := strings.Split(body, "\n")
	if len(bodyLines) > bodyHeight {
		bodyLines = bodyLines[:bodyHeight]
	}
	for len(bodyLines) < bodyHeight {
		bodyLines = append(bodyLines, "")
	}
	lines := make([]string, 0, m.height)
	lines = append(lines, header...)
	lines = append(lines, bodyLines...)
	notice := ""
	if m.notice != "" {
		notice = " " + m.styles.Of(m.noticeKind).Render(m.notice)
	}
	lines = append(lines, notice, " "+keys)
	clip := m.styles.R.NewStyle().MaxWidth(m.width)
	for i, l := range lines {
		lines[i] = clip.Render(render.ForDisplayANSI(l))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) bodyHeight(headerLines int) int {
	return max(1, m.height-headerLines-2)
}

func (m *Model) helpView() string {
	var keys []string
	switch m.view {
	case viewList:
		keys = []string{"j/k, up/down  move", "enter         open the finding", "tab           collapse or expand the summary", "p             publish the review", "q             quit; every decision is already saved"}
	case viewDetail:
		keys = []string{"a    accept (included findings only)", "x    exclude", "s    send back with a note", "u    restore an excluded finding", "r/d  resolve or dismiss the finding's open note", "f    file diff", "n/N  next or previous finding", "j/k  scroll the finding", "J/K  scroll the hunk", "esc  back to the list"}
	case viewFileDiff:
		keys = []string{"j/k, up/down  move", "]/[           next or previous finding", "enter         open the finding on this line", "esc           back to the finding"}
	case viewAction, viewInline:
		keys = []string{"j/k, up/down  move", "enter         choose", "esc           back"}
	}
	keys = append(keys, "?             close this help", "ctrl+c        quit")
	return m.frame([]string{m.styles.Bold.Render("Keys")}, strings.Join(keys, "\n"), "? or esc closes help")
}

func (m *Model) header() string {
	return fmt.Sprintf("%s/%s#%d  round %d  %s", m.target.Owner, m.target.Repo, m.target.Number, m.target.Round, render.ForDisplay(render.OneLine(m.target.Title)))
}

// band is the header line of every view: the program, the run it is reviewing, what the view is showing, and the
// readiness pill at the right edge, which never truncates.
func (m *Model) band(middle string) string {
	round := fmt.Sprintf("round %d", m.target.Round)
	if m.width < wideWidth {
		round = fmt.Sprintf("r%d", m.target.Round)
	}
	left := m.styles.Brand() + " " + m.styles.Accent.Render(m.ref()) + " " + m.styles.Dim.Render(round)
	if middle != "" {
		left += "  " + middle
	}
	return m.styles.Band(left, m.readinessPill(), m.width)
}

func (m *Model) ref() string {
	return fmt.Sprintf("%s/%s#%d", m.target.Owner, m.target.Repo, m.target.Number)
}

func (m *Model) readinessPill() string {
	r := draft.ReadinessOf(m.draft)
	return m.styles.ReadinessPill(r.Ready, len(r.Pending), len(r.OpenNotes))
}

// titleBand is the run title, which is the first thing the band gives up when the window narrows.
func (m *Model) titleBand() string {
	return render.ForDisplay(render.OneLine(m.target.Title))
}

func (m *Model) countsLine() string { return countsLine(m.draft, m.styles) }

// say sets the notice and how it reads: Good for something recorded, Warn for a refusal or a dead end.
func (m *Model) say(kind style.Kind, text string) { m.notice, m.noticeKind = text, kind }

func countsLine(d *draft.Draft, s style.Style) string {
	r := draft.ReadinessOf(d)
	return s.Counts(len(r.Accepted), len(r.Pending), len(r.Excluded), len(r.Withdrawn), len(r.OpenNotes))
}

func locationText(f draft.Finding) string {
	if f.Location == nil {
		return "general"
	}
	return render.ForDisplay(formatLocation(f.Location.Path, f.Location.Line, f.Location.StartLine, f.Location.Side))
}

// formatLocation is path:line, or path:start-end for a range, marked (old) on the left side.
func formatLocation(path string, line, startLine int, side string) string {
	loc := fmt.Sprintf("%s:%d", path, line)
	if startLine != 0 && startLine != line {
		loc = fmt.Sprintf("%s:%d-%d", path, startLine, line)
	}
	if side == draft.SideLeft {
		loc += " (old)"
	}
	return loc
}

func chips(f draft.Finding, disposition string) []string {
	out := []string{disposition}
	if f.Blocking {
		out = append(out, "blocking")
	}
	if f.Label != "" {
		out = append(out, render.ForDisplay(render.OneLine(f.Label)))
	}
	if f.Confidence != "" {
		out = append(out, "confidence "+render.ForDisplay(f.Confidence))
	}
	if f.Severity != "" {
		out = append(out, "severity "+render.ForDisplay(render.OneLine(f.Severity)))
	}
	return out
}

func findingIndex(d *draft.Draft, id string) int {
	for i, f := range d.Findings {
		if f.ID == id {
			return i
		}
	}
	return -1
}

func firstOpenNote(d *draft.Draft, findingID string) (draft.Note, bool) {
	for _, n := range d.Notes {
		if n.FindingID == findingID && n.Status == draft.NoteOpen {
			return n, true
		}
	}
	return draft.Note{}, false
}

// hunkView is nil for a general finding.
func hunkView(dif *diff.Diff, f draft.Finding) ([]diff.ViewLine, error) {
	if f.Location == nil {
		return nil, nil
	}
	return dif.HunkView(f.Location.Path, f.Location.Side, f.Location.Line, f.Location.StartLine, hunkContext)
}

func diffLineText(l diff.ViewLine) string {
	if l.Separator {
		return l.Text
	}
	num := func(n int) string {
		if n == 0 {
			return ""
		}
		return fmt.Sprint(n)
	}
	sign := " "
	switch l.Kind {
	case diff.Add:
		sign = "+"
	case diff.Delete:
		sign = "-"
	}
	return fmt.Sprintf("%4s %4s %s%s", num(l.OldNum), num(l.NewNum), sign, render.ForDisplay(l.Text))
}

func (m *Model) styleDiffLine(l diff.ViewLine, text string) string {
	switch {
	case l.Separator:
		return m.styles.Accent.Render(text)
	case l.Kind == diff.Add:
		return m.styles.Added.Render(text)
	case l.Kind == diff.Delete:
		return m.styles.Removed.Render(text)
	}
	return text
}
