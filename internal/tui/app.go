package tui

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/publish"
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

	// helpTop is the first help line shown when help is taller than the window.
	helpTop int

	openID string
	body   viewport.Model
	noting bool
	note   textarea.Model
	// editing is true while the label and blocking editor is open; editPick indexes editLabels.
	editing      bool
	editLabels   []string
	editPick     int
	editBlocking bool
	glamour      *glamour.TermRenderer
	wrapWidth    int
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
	// settling is true right after a decision moved the view; decision keys are dropped until the new finding has
	// had time to be seen.
	settling bool
	// opening is true from the program's start until settleOnOpen has passed; every key but ctrl+c is dropped.
	opening bool
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
	note := textarea.New()
	note.ShowLineNumbers, note.MaxHeight = false, 0
	note.FocusedStyle, note.BlurredStyle = textarea.Style{}, textarea.Style{}
	note.KeyMap.InsertNewline.SetEnabled(false)
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
	m.cursor = initialCursor(d)
	if st.Color {
		m.darkBackground = st.R.HasDarkBackground()
	}
	return m, nil
}

// Err is the failure that ended the program, if any; refusals during review are notices, not errors.
func (m *Model) Err() error { return m.err }

// settleOnOpen is how long keys are dropped after the program starts: a multiplexer can open review in a split that
// takes focus while the human is still typing into the pane beside it. It is shorter than reading the list. Tests set
// it to zero.
var settleOnOpen = 500 * time.Millisecond

type openedMsg struct{}

func (m *Model) Init() tea.Cmd {
	if settleOnOpen <= 0 {
		return nil
	}
	m.opening = true
	return tea.Tick(settleOnOpen, func(time.Time) tea.Msg { return openedMsg{} })
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, m.fail(m.layout())
	case previewMsg:
		title := ConfirmTitle(m.ref(), m.action, publish.InlineModes[m.pick], len(msg.preview.Comments))
		title.inFlow = true
		m.confirm, m.view = newConfirmation(msg.preview, title), viewConfirm
		return m, nil
	case publishDone:
		return m, m.publishFinished(msg)
	case settledMsg:
		m.settling = false
		return m, nil
	case openedMsg:
		m.opening = false
		return m, nil
	case tea.KeyMsg:
		if m.opening && msg.Type != tea.KeyCtrlC {
			return m, nil
		}
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
		if m.editing {
			return m, m.updateEdit(msg)
		}
		if m.help {
			m.updateHelp(msg)
			return m, nil
		}
		if msg.String() == "?" {
			m.help, m.helpTop = true, 0
			return m, nil
		}
		// The help overlay promises q everywhere; the pickers and the confirmation keep their own keys because esc
		// there means back, not out.
		if msg.String() == "q" && (m.view == viewList || m.view == viewDetail || m.view == viewFileDiff) {
			return m, tea.Quit
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
		keys := m.footer([]style.Hint{{Key: "ctrl+c", Verb: "quit"}})
		if m.sending {
			keys = m.styles.Dim.Render("keys are ignored until the outcome is recorded")
		}
		return m.frame([]string{m.listHeader(), m.headerRule()}, "", keys)
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
	if m.noting {
		m.sizeNote(0)
	}
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

// clipToWindow is the frame's last guard against a line wider than the window. Tests turn it off to see whether any
// view relies on it.
var clipToWindow = true

// maxNoticeLines bounds how far a wrapped refusal may push the body up; longer ones are clipped.
const maxNoticeLines = 3

// frame is the one layout every view renders through: header lines, a body clipped or padded to the rows left, the
// notice and the key line, each clipped to the window width.
func (m *Model) frame(header []string, body, keys string) string {
	wrapped := m.noticeLines()
	for i, l := range wrapped {
		wrapped[i] = m.styles.Of(m.noticeKind).Render(l)
	}
	return m.frameWith(header, body, strings.Join(wrapped, "\n"), keys)
}

// noticeLines is the notice as the frame will print it: wrapped, capped, one empty line when there is none. Every
// body is sized against it, so a notice that wraps takes rows from the body rather than hiding its last rows.
func (m *Model) noticeLines() []string {
	if m.notice == "" {
		return []string{""}
	}
	// A refusal names two SHAs and a fix; on one clipped line the fix is the part that vanishes.
	wrapped := strings.Split(m.styles.Wrap(m.notice, max(20, m.width-1), " "), "\n")
	if len(wrapped) > maxNoticeLines {
		wrapped = wrapped[:maxNoticeLines]
	}
	return wrapped
}

// frameWith is frame with the notice line spelled out, for a view that puts an input there instead.
func (m *Model) frameWith(header []string, body, notice, keys string) string {
	noticeLines := strings.Split(notice, "\n")
	bodyHeight := max(0, m.height-len(header)-len(noticeLines)-1)
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
	lines = append(lines, noticeLines...)
	lines = append(lines, " "+keys)
	clip := m.styles.R.NewStyle().MaxWidth(m.width)
	for i, l := range lines {
		lines[i] = render.ForDisplayANSI(l)
		if clipToWindow {
			lines[i] = clip.Render(lines[i])
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) bodyHeight(headerLines int) int {
	return max(1, m.height-headerLines-len(m.noticeLines())-1)
}

// helpKey is one row of help: the primary key, what it does, and the compatibility alias when there is one.
type helpKey struct {
	Keys, Verb, Alias string
}

// helpSection is one view's keys under the name of the view they belong to.
type helpSection struct {
	title string
	keys  []helpKey
}

func (m *Model) helpSections() map[view]helpSection {
	g := m.glyphs
	upDown, leftRight := g.Up+"/"+g.Down, g.Left+"/"+g.Right
	return map[view]helpSection{
		viewList: {"Finding list", []helpKey{
			{upDown, "move", "k/j"},
			{"enter", "open the finding", ""},
			{"tab", "expand or collapse the summary", ""},
			{"p", "publish once the review is ready", ""},
		}},
		viewDetail: {"Finding detail", []helpKey{
			{leftRight, "previous / next finding", "N/n"},
			{upDown, "scroll the finding", "k/j"},
			{"PgUp/PgDn", "scroll one page", ""},
			{"space", "scroll one page down", ""},
			{"a", "accept; resolves its open notes", ""},
			{"x", "exclude; dismisses its open notes", ""},
			{"s", "send it back with a note", ""},
			{"e", "edit label and blocking", ""},
			{"u", "restore an excluded finding", ""},
			{"r / d", "resolve / dismiss its open note", ""},
			{"f", "whole-file diff", ""},
			{"esc", "back to the list", ""},
		}},
		viewAction: {"Publish steps", []helpKey{
			{upDown, "move", "k/j"},
			{"enter", "choose and go on", ""},
			{"esc", "back one step", ""},
		}},
		viewFileDiff: {"File diff", []helpKey{
			{upDown, "move one line", "k/j"},
			{leftRight, "previous / next marker", "[/]"},
			{"enter", "open the finding on this line", ""},
			{"esc", "back to the finding", ""},
		}},
	}
}

var everywhere = helpSection{"Everywhere", []helpKey{
	{"?", "toggle this help", ""},
	{"q", "quit; decisions are saved", ""},
	{"ctrl+c", "quit", ""},
}}

// helpView leads with the keys of the view it was opened from, then the ones that work everywhere and the other
// views: in two columns when every row fits one, otherwise in one column that scrolls.
func (m *Model) helpView() string {
	lines := m.helpLines()
	height := m.bodyHeight(helpHeaderLines)
	top := min(m.helpTop, max(0, len(lines)-height))
	right := m.readiness()
	var hints []style.Hint
	if len(lines) > height {
		right = m.styles.Dim.Render(fmt.Sprintf("lines %d%s%d of %d", top+1, m.sign("\u2013", "-"), min(top+height, len(lines)), len(lines)))
		hints = append(hints, style.Hint{Key: m.glyphs.Up + "/" + m.glyphs.Down, Verb: "scroll", Role: style.RoleNav})
	}
	hints = append(hints, style.Hint{Key: "? or esc", Verb: "close help", Role: style.RoleHelp})
	header := m.styles.Header([]style.HeaderPart{
		{Text: strings.TrimSpace(m.glyphs.Help + " Keys"), Bold: true},
		{Text: "from " + strings.ToLower(m.helpCurrent().title), Drop: 1, Kind: style.Dim},
	}, right, m.width)
	return m.frame([]string{header, m.headerRule()}, strings.Join(lines[top:], "\n"), m.footer(hints))
}

// helpCurrentView is the view whose keys help leads with; both publish steps share one section.
func (m *Model) helpCurrentView() view {
	if m.view == viewInline {
		return viewAction
	}
	if _, ok := m.helpSections()[m.view]; !ok {
		return viewList
	}
	return m.view
}

func (m *Model) helpCurrent() helpSection { return m.helpSections()[m.helpCurrentView()] }

func (m *Model) helpLines() []string {
	sections := m.helpSections()
	current := m.helpCurrentView()
	rest := []helpSection{everywhere}
	for _, v := range []view{viewList, viewDetail, viewFileDiff} {
		if v != current {
			rest = append(rest, sections[v])
		}
	}
	all := append([]helpSection{sections[current]}, rest...)

	keyWidth, widest := 0, 0
	for _, section := range all {
		for _, k := range section.keys {
			keyWidth = max(keyWidth, style.Width(k.Keys))
		}
	}
	for _, section := range all {
		for _, k := range section.keys {
			widest = max(widest, style.Width(m.helpRow(k, keyWidth)))
		}
	}

	// Columns are four cells apart, after the one-cell margin.
	const helpGap = 4
	content := style.Content(m.width)
	column := (content - 1 - helpGap) / 2
	var body []string
	if column >= 30 && widest <= column {
		left, right := m.helpColumn(all[:1], keyWidth), m.helpColumn(rest, keyWidth)
		for i := range max(len(left), len(right)) {
			row := " "
			if i < len(left) {
				row += style.Pad(left[i], column)
			} else {
				row += strings.Repeat(" ", column)
			}
			if i < len(right) {
				row += strings.Repeat(" ", helpGap) + right[i]
			}
			body = append(body, strings.TrimRight(row, " "))
		}
	} else {
		for _, l := range m.helpColumn(all, keyWidth) {
			body = append(body, strings.TrimRight(" "+m.styles.TruncRight(l, content-1), " "))
		}
	}
	lines := append(body, "")
	stale := m.styles.Wrap("Decisions are recorded against the draft version on screen. If the draft changed meanwhile, "+
		"nothing is recorded and the current version is shown instead.", content-1, " ")
	for _, l := range strings.Split(stale, "\n") {
		lines = append(lines, m.styles.Dim.Render(l))
	}
	return lines
}

// helpColumn lays sections out one under another, a blank line between them.
func (m *Model) helpColumn(sections []helpSection, keyWidth int) []string {
	var out []string
	for i, section := range sections {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, m.styles.Bold.Render(section.title))
		for _, k := range section.keys {
			out = append(out, m.helpRow(k, keyWidth))
		}
	}
	return out
}

func (m *Model) helpRow(k helpKey, keyWidth int) string {
	row := m.styles.Accent.Bold(true).Render(style.Pad(k.Keys, keyWidth)) + "  " + k.Verb
	if k.Alias != "" {
		row += "  " + m.styles.Dim.Render("alias "+k.Alias)
	}
	return row
}

func (m *Model) updateHelp(msg tea.KeyMsg) {
	lines, height := len(m.helpLines()), m.bodyHeight(helpHeaderLines)
	last := max(0, lines-height)
	m.helpTop = min(m.helpTop, last)
	switch msg.String() {
	case "?", "esc", "q":
		m.help = false
	case "j", "down":
		m.helpTop = min(m.helpTop+1, last)
	case "k", "up":
		m.helpTop = max(m.helpTop-1, 0)
	case "pgdown", " ":
		m.helpTop = min(m.helpTop+height, last)
	case "pgup":
		m.helpTop = max(m.helpTop-height, 0)
	}
}

// helpHeaderLines is the header and its rule.
const helpHeaderLines = 2

// headerRule closes the header block of every screen but the confirmation, whose body opens with titled rules. It
// is dim, so it reads as an edge rather than a pane border.
func (m *Model) headerRule() string { return m.styles.Rule(m.width, "", "") }

// header is the flat line every view opens with, readiness at its right edge.
func (m *Model) header(parts ...style.HeaderPart) string {
	return m.styles.Header(parts, m.readiness(), m.width)
}

// listHeader names the program, the run and its title; the title gives way first, then the brand and the round.
func (m *Model) listHeader() string {
	return m.header(
		style.HeaderPart{Text: "loupe", Drop: 3, Bold: true},
		// A ref too long for the window keeps its pull request number.
		style.HeaderPart{Text: m.prRef(), Squeeze: true, Left: true},
		style.HeaderPart{Text: fmt.Sprintf("round %d", m.target.Round), Drop: 2, Kind: style.Dim},
		style.HeaderPart{Text: m.titleText(), Drop: 4, Trunc: true, Kind: style.Dim},
	)
}

// footer fits hints to the window, leaving the one-cell margin at each end.
func (m *Model) footer(hints []style.Hint) string {
	line, _ := m.styles.Footer(hints, m.width-2)
	return line
}

// prRef is the run reference behind the pull request icon in the tier that has one.
func (m *Model) prRef() string { return strings.TrimSpace(m.glyphs.PR + " " + m.ref()) }

func (m *Model) ref() string {
	return fmt.Sprintf("%s/%s#%d", m.target.Owner, m.target.Repo, m.target.Number)
}

func (m *Model) readiness() string {
	r := draft.ReadinessOf(m.draft)
	return m.styles.Readiness(r.Ready, len(r.Pending), len(r.OpenNotes))
}

func (m *Model) titleText() string {
	return render.ForDisplay(render.OneLine(m.target.Title))
}

// countsLines is the tally under the list header, on the one-cell margin.
func (m *Model) countsLines() []string {
	lines := countsLines(m.draft, m.styles, m.width-1)
	for i, l := range lines {
		lines[i] = " " + l
	}
	return lines
}

// sign is a character that has a typographic form and an ASCII one; the glyph set decides which the locale can print.
func (m *Model) sign(unicode, ascii string) string {
	if m.glyphs.Ellipsis == "\u2026" {
		return unicode
	}
	return ascii
}

// say sets the notice and how it reads: Good for something recorded, Warn for a refusal or a dead end.
func (m *Model) say(kind style.Kind, text string) { m.notice, m.noticeKind = text, kind }

func countsLines(d *draft.Draft, s style.Style, width int) []string {
	r := draft.ReadinessOf(d)
	return s.Counts(len(r.Accepted), len(r.Pending), len(r.Excluded), len(r.Withdrawn), len(r.OpenNotes), width)
}

// initialCursor selects what needs the human first: the first pending finding, then the first with an open note.
func initialCursor(d *draft.Draft) int {
	r := draft.ReadinessOf(d)
	for _, ids := range [][]string{r.Pending, openNoteFindings(d)} {
		for i, f := range d.Findings {
			if slices.Contains(ids, f.ID) {
				return i
			}
		}
	}
	return 0
}

func openNoteFindings(d *draft.Draft) []string {
	var out []string
	for _, n := range d.Notes {
		if n.Status == draft.NoteOpen {
			out = append(out, n.FindingID)
		}
	}
	return out
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
// placeholder. general adds the chip for a finding with no location, where nothing else on screen says so.
func chips(s style.Style, f draft.Finding, disposition string, general bool) []chip {
	glyph, word, kind := s.Disposition(disposition)
	out := []chip{{glyph, word, kind}}
	if f.Blocking {
		out = append(out, chip{s.Glyphs.Blocking, "blocking", style.Bad})
	}
	if general && f.Location == nil {
		out = append(out, chip{"", "general", style.Dim})
	}
	if f.Label != "" {
		out = append(out, chip{"", render.ForDisplay(render.OneLine(f.Label)), style.Plain})
	}
	if f.Confidence != "" {
		out = append(out, chip{"", "confidence " + render.ForDisplay(f.Confidence), style.Dim})
	}
	if f.Severity != "" {
		out = append(out, chip{"", "severity " + render.ForDisplay(render.OneLine(f.Severity)), style.Dim})
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

// decidedNotice names the decision and the notes it closed, so a note never closes without the human seeing it.
func decidedNotice(sep, glyph, findingID, decision string, closed []string, noteStatus string) string {
	out := glyph + " " + findingID + " " + decision
	if len(closed) > 0 {
		out += " " + sep + " " + strings.Join(closed, ", ") + " " + noteStatus
	}
	return out
}

// hunkView is nil for a general finding.
func hunkView(dif *diff.Diff, f draft.Finding) ([]diff.ViewLine, error) {
	if f.Location == nil {
		return nil, nil
	}
	return dif.HunkView(f.Location.Path, f.Location.Side, f.Location.Line, f.Location.StartLine, hunkContext)
}

// diffRow is one line of a diff: its number on the new side, a gutter wide enough for the widest marker, and the
// line as the diff carries it.
func diffRow(l diff.ViewLine, gutter string, gutterWidth int) string {
	if l.Separator {
		return render.ForDisplay(l.Text)
	}
	lead, text := diffCells(l)
	return lead + style.Pad(gutter, gutterWidth) + text
}

// diffCells splits a diff line around its gutter, so a painted row can color the gutter on its own. A deleted line
// has no new-side number, so it keeps the old one.
func diffCells(l diff.ViewLine) (lead, text string) {
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
	return fmt.Sprintf("%5s  ", number), "  " + sign + render.ForDisplay(l.Text)
}

// anchoredRow is the diff line a finding points at: the whole row on the anchor background, the marker in the warn
// color, painted piece by piece because a reset inside the row would end the band.
func (m *Model) anchoredRow(l diff.ViewLine) string {
	lead, text := diffCells(l)
	if !m.styles.Color {
		return " " + lead + m.glyphs.Anchor + text
	}
	bg, marker := m.styles.Anchor, m.styles.On(m.styles.Anchor, m.styles.Warn.Bold(true))
	rest := style.Pad(text, max(0, style.Content(m.width)-2-style.Width(lead)))
	return bg.Render(" "+lead) + marker.Render(m.glyphs.Anchor) + bg.Render(rest)
}

func (m *Model) styleDiffLine(l diff.ViewLine, text string) string {
	if l.Separator {
		return m.styles.Accent.Render(text)
	}
	return m.diffStyle(l).Render(text)
}

// diffStyle is the color of a diff line by what it does to the file.
func (m *Model) diffStyle(l diff.ViewLine) lipgloss.Style {
	switch l.Kind {
	case diff.Add:
		return m.styles.Added
	case diff.Delete:
		return m.styles.Removed
	}
	return m.styles.R.NewStyle()
}
