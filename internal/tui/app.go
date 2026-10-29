package tui

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"

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
	viewPublished
)

type Config struct {
	Dir    string
	Getenv func(string) string
	Now    func() time.Time
	// Output is where the program draws; it decides which colors the terminal supports.
	Output io.Writer
	// GitHub is called only when the human publishes, so reviewing never needs credentials.
	GitHub func() (github.Client, error)
	// OpenURL opens the published review in a browser; nil offers no o on the published screen.
	OpenURL func(string) error
	// Screen is set only by a test, to stand in for a terminal.
	Screen *Screen
}

// Screen is a test's stand-in for a terminal, which bubbletea cannot ask for a size. The program starts at its size
// and drops no keys as it opens or moves to the next finding: those pauses guard a human's typeahead, and a test
// types only once the screen shows what it waits for.
type Screen struct{ Width, Height int }

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
	// noticeGen is the notice's generation, so the timer of a success notice clears only its own.
	noticeGen int
	// flashGen is the notice generation still flashing as a solid tab, in flashTone; 0 when none is.
	flashGen  int
	flashTone style.Tone
	err       error

	// order is the draft's findings as every surface presents them; the cursor and the detail's position index it.
	order  []draft.Finding
	cursor int
	// summaryCollapsed is the human's choice. A short window collapses the summary on its own without changing it, so
	// the choice comes back when the window grows.
	summaryCollapsed bool

	// helpTop is the first help line shown when help is taller than the window.
	helpTop int

	openID string
	body   viewport.Model
	noting bool
	note   textarea.Model
	// keptNotes holds, per finding, text the human typed into a note and then left without it being recorded, so the
	// next send-back on that finding starts from it. It lives only as long as the program.
	keptNotes map[string]string
	// editing is true while the label and blocking editor is open; editPick indexes editLabels.
	editing      bool
	editLabels   []string
	editPick     int
	editBlocking bool
	glamour      *glamour.TermRenderer
	wrapWidth    int
	// tokensKey and tokens are the last highlighted lines, so moving or redrawing never tokenizes them again.
	tokensKey string
	tokens    [][]style.CodeToken
	// Only the first background response selects the palette for this program.
	backgroundReceived bool

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
	// message is the review's opening as the human last left it, kept for the life of this program so a cancel or
	// a refusal does not make them write it again. It reaches no file, and nothing but the confirmation sets it.
	message string
	// settling is true right after a decision moved the view; decision keys are dropped until the new finding has
	// had time to be seen.
	settling bool
	// opening is true from the program's start until settleOnOpen has passed; every key but ctrl+c is dropped.
	opening bool
	// polling is true while a poll tick is in flight, so there is never more than one.
	polling bool
	// elsewhere names what an outside process changed in other findings, found when a decision is recorded.
	elsewhere string
	// pollSkipped is true when a tick found the human writing or confirming; the draft is re-read as they leave.
	pollSkipped bool
	// published is the URL of the review this program sent. browsing is set while the opener runs, for the view in
	// browseFrom, and openFailed once it failed.
	published  string
	browsing   bool
	browseFrom view
	openFailed bool
	// browseID ties the opener's answer to the request that started it, so a late answer from list or detail cannot
	// quit the program or reach a view the human has since left.
	browseID int
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
	st.UseReviewTheme()
	note := textarea.New()
	note.ShowLineNumbers, note.MaxHeight = false, 0
	note.SetStyles(textarea.Styles{})
	note.KeyMap.InsertNewline.SetEnabled(false)
	m := &Model{
		cfg:       cfg,
		target:    target,
		diff:      parsed,
		glyphs:    st.Glyphs,
		styles:    st,
		width:     80,
		height:    24,
		note:      note,
		keptNotes: map[string]string{},
		body:      viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		file:      viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
	}
	if cfg.Screen != nil {
		m.width, m.height = cfg.Screen.Width, cfg.Screen.Height
	}
	m.setDraft(d)
	m.cursor = initialCursor(d, m.order)

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
	if settleOnOpen <= 0 || m.cfg.Screen != nil {
		return tea.Batch(tea.RequestBackgroundColor, m.schedulePoll())
	}
	m.opening = true
	return tea.Batch(tea.RequestBackgroundColor, tea.Tick(settleOnOpen, func(time.Time) tea.Msg { return openedMsg{} }), m.schedulePoll())
}

// pollInterval is how often the draft is re-read.
var pollInterval = 2 * time.Second

type pollMsg struct{}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	gen := m.noticeGen
	if _, ok := msg.(pollMsg); ok {
		m.polling = false
		if m.pollHeld() {
			m.pollSkipped = true
		} else {
			cmd = m.checkDraft(false)
		}
	} else {
		_, cmd = m.update(msg)
	}
	if m.pollSkipped && !m.pollHeld() && m.err == nil {
		m.pollSkipped = false
		cmd = tea.Batch(cmd, m.checkDraft(false))
	}
	// The detail's viewport follows its editor and notice rows, which any update can open, close or change.
	if m.view == viewDetail && !tooSmall(m.width, m.height) && m.err == nil {
		m.sizeDetail()
	}
	cmd = tea.Batch(cmd, m.schedulePoll(), m.expireNotice(gen))
	return m, cmd
}

// pollHeld is true while redrawing would move something the human is writing or confirming.
func (m *Model) pollHeld() bool {
	return m.noting || m.editing || m.sending || (m.view != viewList && m.view != viewDetail)
}

func (m *Model) schedulePoll() tea.Cmd {
	if m.polling || m.err != nil {
		return nil
	}
	m.polling = true
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollMsg{} })
}

// checkDraft shows the draft as it is on disk and says what another process changed. asked is true for the reload
// key, which answers even when nothing changed.
func (m *Model) checkDraft(asked bool) tea.Cmd {
	changed, err := m.reload()
	if err != nil {
		return m.fail(err)
	}
	switch {
	case changed != "":
		m.say(style.Note, m.changedNotice(changed))
	case asked:
		m.say(style.Dim, "draft is current")
	}
	return nil
}

func (m *Model) changedNotice(changed string) string {
	return strings.TrimSpace(m.glyphs.Note + " " + changed)
}

// reload shows the draft as it is on disk, keeping the cursor on its finding and the open finding where the human
// had scrolled it, and returns what another process changed, or "" when the draft is as displayed. Every write the
// human makes moves the displayed version, so a difference is never theirs.
func (m *Model) reload() (string, error) {
	d, err := draft.Load(m.cfg.Dir)
	if err != nil {
		return "", err
	}
	if d.Version == m.version {
		return "", nil
	}
	changed := changeNotice(m.draft, d, "")
	id, offset := "", m.body.YOffset()
	if len(m.order) > 0 {
		id = m.order[m.cursor].ID
	}
	m.setDraft(d)
	if j := orderedIndex(m.order, id); j >= 0 {
		m.cursor = j
	}
	m.cursor = max(min(m.cursor, len(m.order)-1), 0)
	if m.view == viewDetail {
		if err := m.refreshDetail(); err != nil {
			return "", err
		}
		m.body.SetYOffset(offset)
	}
	return changed, nil
}

// draftChanged is changeNotice's answer when nothing it names changed.
const draftChanged = "draft changed"

// changeNotice names what another process changed between two versions of the draft, in one line, leaving out the
// finding the human just decided.
func changeNotice(before, after *draft.Draft, decided string) string {
	var parts []string
	replied := map[string]bool{}
	for _, r := range before.Replies {
		replied[r.ID] = true
	}
	noteFinding := map[string]string{}
	for _, n := range after.Notes {
		noteFinding[n.ID] = n.FindingID
	}
	for _, r := range after.Replies {
		if !replied[r.ID] && noteFinding[r.NoteID] != decided {
			parts = append(parts, fmt.Sprintf("%s answered on %s", r.NoteID, noteFinding[r.NoteID]))
		}
	}
	status := map[string]string{}
	for _, n := range before.Notes {
		status[n.ID] = n.Status
	}
	for _, n := range after.Notes {
		was, ok := status[n.ID]
		switch {
		case n.FindingID == decided:
		case !ok:
			parts = append(parts, fmt.Sprintf("%s sent back on %s", n.ID, n.FindingID))
		case was != n.Status && n.Status != draft.NoteOpen:
			parts = append(parts, n.ID+" closed")
		}
	}
	shown := map[string]draft.Finding{}
	for _, f := range before.Findings {
		shown[f.ID] = f
	}
	for _, f := range after.Findings {
		was, ok := shown[f.ID]
		switch {
		case f.ID == decided:
		case !ok:
			parts = append(parts, f.ID+" filed")
		case was.Rev != f.Rev || was.Included != f.Included:
			parts = append(parts, f.ID+" changed")
		case before.Decisions[f.ID].Decision != after.Decisions[f.ID].Decision:
			parts = append(parts, f.ID+" decided")
		}
	}
	if before.Summary != after.Summary {
		parts = append(parts, "summary changed")
	}
	if len(parts) == 0 {
		return draftChanged
	}
	return strings.Join(parts, ", ")
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		if !m.backgroundReceived {
			m.backgroundReceived = true
			m.styles.SetDarkBackground(msg.IsDark())
			m.glamour = nil
			if !tooSmall(m.width, m.height) {
				return m, m.fail(m.layout())
			}
		}
		return m, nil
	case tea.PasteMsg:
		if m.opening || tooSmall(m.width, m.height) || m.sending {
			return m, nil
		}
		if m.view == viewConfirm && m.confirm.typing {
			cmd := m.confirm.typeMessage(msg)
			m.confirm.sync(m)
			return m, cmd
		}
		if m.noting {
			return m, m.typeNote(msg)
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Nothing is laid out while too small; the resize that ends it lays the view out again.
		if tooSmall(m.width, m.height) {
			return m, nil
		}
		return m, m.fail(m.layout())
	case previewMsg:
		title := ConfirmTitle(m.ref(), m.action, publish.InlineModes[m.pick], len(msg.preview.Comments))
		m.confirm, m.view = newConfirmation(msg.preview, title, m.message), viewConfirm
		return m, nil
	case publishDone:
		return m, m.publishFinished(msg)
	case settledMsg:
		m.settling = false
		return m, nil
	case openedMsg:
		m.opening = false
		return m, nil
	case noticeExpired:
		if msg.gen == m.noticeGen {
			m.notice = ""
		}
		return m, nil
	case flashEnded:
		if msg.gen == m.flashGen {
			m.flashGen = 0
		}
		return m, nil
	case browserMsg:
		return m, m.browserDone(msg)
	case tea.KeyPressMsg:
		// The settle comes before the too-small screen, so a q typed for the agent's pane cannot quit review.
		if m.opening && msg.String() != "ctrl+c" {
			return m, nil
		}
		if tooSmall(m.width, m.height) {
			var ok bool
			if msg, ok = smallKey(msg); !ok {
				return m, nil
			}
		}
		// The confirmation comes first so that ctrl+c, like every key but y, p and the toggles, declines.
		if m.view == viewConfirm {
			return m, m.updateConfirm(msg)
		}
		if m.sending {
			return m, nil
		}
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.view == viewPublishing {
			return m, nil
		}
		if m.view == viewPublished {
			return m, m.updatePublished(msg)
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

func (m *Model) View() tea.View {
	v := tea.NewView(m.content())
	v.AltScreen = true
	return v
}

func (m *Model) content() string {
	if tooSmall(m.width, m.height) {
		return m.smallView()
	}
	if m.help {
		return m.helpOverlay(m.viewContent())
	}
	return m.viewContent()
}

// viewContent is the current view without help over it.
func (m *Model) viewContent() string {
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
		return m.publishingView()
	case viewPublished:
		return m.publishedView()
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

// setDraft takes a freshly loaded draft and rebuilds the order every surface reads, so the two can never disagree.
func (m *Model) setDraft(d *draft.Draft) {
	m.draft, m.version, m.order = d, d.Version, draft.Ordered(d)
}

// Decide applies fn to the finding as displayed and reports whether it was recorded. A refusal reloads the draft and
// becomes the notice, so the human sees the finding as it now is. Other findings the draft brings with it are named
// in m.elsewhere for the caller's notice.
func (m *Model) Decide(findingID string, fn func(*draft.Draft) error) (bool, error) {
	displayed := m.draft
	d, notice, err := decide(m.cfg.Dir, displayed, findingID, m.cfg.Getenv, fn)
	if err != nil {
		return false, err
	}
	m.elsewhere = ""
	m.setDraft(d)
	m.elsewhere = elsewhereNotice(displayed, d, findingID)
	if notice == "" {
		return true, nil
	}
	m.sayDecided(notice)
	m.noticeKind = style.Warn
	return false, nil
}

// elsewhereNotice names what changed besides the decided finding between the draft the human saw and the one the
// decision returned, or "". The decision's own write is on the decided finding, so it is never named here.
func elsewhereNotice(displayed, next *draft.Draft, findingID string) string {
	if changed := changeNotice(displayed, next, findingID); changed != draftChanged {
		return changed
	}
	return ""
}

// sayDecided is the notice for a decision, followed by whatever changed elsewhere in the same step.
func (m *Model) sayDecided(text string) {
	text = joinNotice(text, m.elsewhere)
	m.elsewhere = ""
	m.say(style.Good, text)
}

// decide refuses a decision as stale only when its own finding changed since displayed, so the agent writing
// elsewhere never refuses it. It returns the draft to display next and, when nothing was recorded, the notice.
func decide(dir string, displayed *draft.Draft, findingID string, getenv func(string) string, fn func(*draft.Draft) error) (*draft.Draft, string, error) {
	shown, err := draft.FindingState(displayed, findingID)
	if err != nil {
		return nil, "", err
	}
	d, err := draft.Mutate(dir, "review", nil, getenv, func(d *draft.Draft) error {
		stored, err := draft.FindingState(d, findingID)
		if err != nil {
			return err
		}
		if !bytes.Equal(stored, shown) {
			return refusal.New(refusal.Version, findingID+" changed since it was displayed", "review it again as it now is")
		}
		return fn(d)
	})
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

// fill sets a viewport's content and pulls its offset back to the last full page. SetContent clamps only an offset
// past the last line, so a view scrolled to its end in a short window would keep that offset over blank rows once
// the window grows.
func fill(v *viewport.Model, content string) {
	v.SetContent(content)
	v.SetYOffset(v.YOffset())
}

// viewFooterRows is how many rows the current view's own footer takes.
func (m *Model) viewFooterRows() int { return len(m.viewFooter()) }

// viewFooter is the current view's own footer, which its body is sized against.
func (m *Model) viewFooter() []string {
	switch m.view {
	case viewDetail:
		return m.detailFooter()
	case viewFileDiff:
		return m.keys(groupHints(m.fileDiffHints())...)
	case viewAction:
		return m.keys(groupHints(m.publishStepHints(actionEnter))...)
	case viewInline:
		return m.keys(groupHints(m.publishStepHints(inlineEnter))...)
	case viewConfirm:
		return m.confirmKeys()
	case viewPublishing:
		if m.sending {
			return []string{m.styles.Overlay().Render("keys are ignored until the outcome is recorded")}
		}
		return m.keys(style.KeyGroup{{Key: "ctrl+c", Verb: "quit"}})
	case viewPublished:
		return m.publishedKeys()
	}
	return m.keys(groupHints(m.listHints())...)
}

// helpKey is one row of help: the primary key, what it does, and the compatibility alias when there is one.
type helpKey struct {
	Keys, Verb, Alias string
}

// helpSection is one view's keys under the name of the view they belong to.
type helpSection struct {
	title, short string
	keys         []helpKey
}

func (m *Model) helpSections() map[view]helpSection {
	g := m.glyphs
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
		viewFileDiff: {"File diff", "file diff", []helpKey{
			{upDown, "move one line", "k/j"},
			{leftRight, "previous / next marker", "[/]"},
			{"enter", "open the finding on this line", ""},
			{"esc", "back to the finding", ""},
		}},
	}
}

var everywhere = helpSection{"Everywhere", "", []helpKey{
	{"?", "toggle this help", ""},
	{"q", "quit; decisions are saved", ""},
	{"ctrl+c", "quit", ""},
}}

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

// helpLines leads with the keys of the view help was opened from, then the ones that work everywhere and the other
// views, then the legend for the progress bar and the confidence scale, in one column the overlay scrolls.
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
	keyWidth := 0
	for _, section := range all {
		for _, k := range section.keys {
			keyWidth = max(keyWidth, style.Width(k.Keys))
		}
	}
	width, _ := style.PaneInner(min(m.width-4, 72), 0)
	lines := append(m.helpColumn(all, keyWidth), "")
	for _, l := range m.legend() {
		lines = append(lines, strings.Split(m.styles.Wrap(l, width, ""), "\n")...)
	}
	lines = append(lines, "")
	stale := m.styles.Wrap("Decisions are recorded against the draft version on screen. If the draft changed meanwhile, "+
		"nothing is recorded and the current version is shown instead.", width, "")
	for _, l := range strings.Split(stale, "\n") {
		lines = append(lines, m.styles.Dim.Render(l))
	}
	return lines
}

// legend explains the progress bar and the confidence scale in the characters this terminal draws them in, and tells
// an open note from one the agent has answered.
func (m *Model) legend() []string {
	s := m.styles
	cell := func(st style.ProgressState) string { return s.Progress([]style.ProgressState{st}, 0) }
	conf := func(word string) string { return s.ConfidenceScale(word) }
	return []string{
		"Progress: " + cell(style.ProgressAccepted) + " accepted, " + cell(style.ProgressExcluded) + " excluded, " +
			cell(style.ProgressNote) + " pending with an open note, " + cell(style.ProgressBlocking) + " pending blocking, " +
			cell(style.ProgressPending) + " pending. Withdrawn findings have no cell.",
		"Confidence: " + conf("high") + " high, " + conf("medium") + " medium, " + conf("low") + " low.",
		"Notes: " + s.NoteMark(false).Render(m.glyphs.Note) + " an open note, " + s.NoteMark(true).Render(m.glyphs.Note) +
			" an open note the agent answered, which waits on you.",
	}
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
	row := m.styles.Emphasis(m.styles.Accent).Render(style.Pad(k.Keys, keyWidth)) + "  " + k.Verb
	if k.Alias != "" {
		row += "  " + m.styles.Dim.Render("alias "+k.Alias)
	}
	return row
}

func (m *Model) updateHelp(msg tea.KeyPressMsg) {
	lines, height := len(m.helpLines()), m.helpHeight()
	last := max(0, lines-height)
	m.helpTop = min(m.helpTop, last)
	switch msg.String() {
	case "?", "esc", "q":
		m.help = false
	case "j", "down":
		m.helpTop = min(m.helpTop+1, last)
	case "k", "up":
		m.helpTop = max(m.helpTop-1, 0)
	case "pgdown", "space":
		m.helpTop = min(m.helpTop+height, last)
	case "pgup":
		m.helpTop = max(m.helpTop-height, 0)
	}
}

func (m *Model) ref() string {
	return fmt.Sprintf("%s/%s#%d", m.target.Owner, m.target.Repo, m.target.Number)
}

func (m *Model) titleText() string {
	return render.ForDisplay(render.OneLine(m.target.Title))
}

// sign is a character that has a typographic form and an ASCII one; the glyph set decides which the locale can print.
func (m *Model) sign(unicode, ascii string) string {
	if m.glyphs.Ellipsis == "\u2026" {
		return unicode
	}
	return ascii
}

// initialCursor selects what needs the human first: the first pending finding, then the first with an open note.
func initialCursor(d *draft.Draft, order []draft.Finding) int {
	r := draft.ReadinessOf(d)
	for _, ids := range [][]string{r.Pending, openNoteFindings(d)} {
		for i, f := range order {
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

func orderedIndex(fs []draft.Finding, id string) int {
	for i, f := range fs {
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

func joinNotice(text, elsewhere string) string {
	if elsewhere == "" {
		return text
	}
	return text + "; " + elsewhere
}
