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
	notice := ""
	if m.notice != "" {
		notice = " " + m.styles.Of(m.noticeKind).Render(m.notice)
	}
	return m.frameWith(header, body, notice, keys)
}

// frameWith is frame with the notice line spelled out, for a view that puts an input there instead.
func (m *Model) frameWith(header []string, body, notice, keys string) string {
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

// helpSection is one view's keys under the name of the view they belong to.
type helpSection struct {
	title string
	keys  []style.Key
}

func helpSections() map[view]helpSection {
	return map[view]helpSection{
		viewList: {"list", []style.Key{
			{K: "j / k", Verb: "move"},
			{K: "enter", Verb: "open the finding"},
			{K: "tab", Verb: "expand or collapse the summary"},
			{K: "p", Verb: "publish, once every included finding is accepted"},
		}},
		viewDetail: {"detail", []style.Key{
			{K: "a", Verb: "accept the finding as shown"},
			{K: "x", Verb: "exclude it from the review"},
			{K: "s", Verb: "send it back with a one-line note"},
			{K: "u", Verb: "restore an excluded finding"},
			{K: "r / d", Verb: "resolve / dismiss its open note"},
			{K: "f", Verb: "whole-file diff"},
			{K: "n / N", Verb: "next / previous finding"},
			{K: "j / k", Verb: "scroll the finding"},
			{K: "J / K", Verb: "scroll the hunk"},
			{K: "esc", Verb: "back to the list"},
		}},
		viewFileDiff: {"file diff", []style.Key{
			{K: "j / k", Verb: "move"},
			{K: "] / [", Verb: "next / previous finding"},
			{K: "enter", Verb: "open the finding on this line"},
			{K: "esc", Verb: "back to the finding"},
		}},
	}
}

var everywhere = helpSection{"everywhere", []style.Key{
	{K: "?", Verb: "toggle this help"},
	{K: "q", Verb: "quit; decisions are already saved"},
	{K: "ctrl+c", Verb: "quit"},
}}

// helpView is a two-column table: the keys of the view it was opened from first, then the ones that work everywhere
// and the other views.
func (m *Model) helpView() string {
	sections := helpSections()
	current := m.view
	if _, ok := sections[current]; !ok {
		// The pickers and the confirmation carry their keys on screen, so their help opens on the list.
		current = viewList
	}
	order := []view{viewList, viewDetail, viewFileDiff}
	left := []helpSection{sections[current]}
	right := []helpSection{everywhere}
	for _, v := range order {
		if v != current {
			right = append(right, sections[v])
		}
	}

	column := max(30, m.width/2-1)
	lines := make([]string, 0, m.height)
	leftLines, rightLines := m.helpColumn(left, column), m.helpColumn(right, column)
	for i := range max(len(leftLines), len(rightLines)) {
		row := " "
		if i < len(leftLines) {
			row += style.Pad(leftLines[i], column)
		} else {
			row += strings.Repeat(" ", column)
		}
		if i < len(rightLines) {
			row += " " + rightLines[i]
		}
		lines = append(lines, strings.TrimRight(row, " "))
	}
	lines = append(lines, "", m.styles.Wrap("Decisions are recorded against the draft version on screen. If the draft changed meanwhile, "+
		"nothing is recorded and the current version is shown instead.", m.width-1, " "))

	keys := m.styles.Keys([]style.Key{{K: "? or esc", Verb: "closes help"}})
	return m.frame([]string{m.styles.Band(m.styles.Brand()+" keys", "", m.width)}, strings.Join(lines, "\n"), keys)
}

// helpColumn lays one column out: every section's keys aligned under its heading.
func (m *Model) helpColumn(sections []helpSection, width int) []string {
	keyWidth := 0
	for _, section := range sections {
		for _, k := range section.keys {
			keyWidth = max(keyWidth, style.Width(k.K))
		}
	}
	var out []string
	for i, section := range sections {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, m.styles.Heading(section.title))
		for _, k := range section.keys {
			out = append(out, m.styles.TruncRight(m.styles.Bold.Render(style.Pad(k.K, keyWidth))+"  "+m.styles.Dim.Render(k.Verb), width))
		}
	}
	return out
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

// sign is a character that has a typographic form and an ASCII one; the glyph set decides which the locale can print.
func (m *Model) sign(unicode, ascii string) string {
	if m.glyphs.Ellipsis == "\u2026" {
		return unicode
	}
	return ascii
}

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

// chip is one state of a finding: a glyph, the word that names it and the color both carry.
type chip struct {
	glyph, text string
	kind        style.Kind
}

// chips are the states a finding is in, in reading order. A field that is not set has no chip, so nothing is a
// placeholder.
func chips(s style.Style, f draft.Finding, disposition string) []chip {
	glyph, word, kind := s.Disposition(disposition)
	out := []chip{{glyph, word, kind}}
	if f.Blocking {
		out = append(out, chip{s.Glyphs.Blocking, "blocking", style.Bad})
	}
	if f.Label != "" {
		out = append(out, chip{"", render.ForDisplay(render.OneLine(f.Label)), style.Plain})
	}
	if f.Confidence != "" {
		out = append(out, chip{"", "confidence " + render.ForDisplay(f.Confidence), style.Faint})
	}
	if f.Severity != "" {
		out = append(out, chip{"", "severity " + render.ForDisplay(render.OneLine(f.Severity)), style.Faint})
	}
	return out
}

func chipRow(s style.Style, cs []chip) string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, s.Chip(c.kind, c.glyph, c.text))
	}
	return strings.Join(out, "   ")
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

// diffRow is one line of a diff: its number on the new side, a gutter wide enough for the widest marker, and the
// line as the diff carries it. A deleted line has no new-side number, so it keeps the old one.
func diffRow(l diff.ViewLine, gutter string, gutterWidth int) string {
	if l.Separator {
		return render.ForDisplay(l.Text)
	}
	num := l.NewNum
	if num == 0 {
		num = l.OldNum
	}
	number := ""
	if num > 0 {
		number = fmt.Sprint(num)
	}
	sign := " "
	switch l.Kind {
	case diff.Add:
		sign = "+"
	case diff.Delete:
		sign = "-"
	}
	return fmt.Sprintf("%5s  %s  %s%s", number, style.Pad(gutter, gutterWidth), sign, render.ForDisplay(l.Text))
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
