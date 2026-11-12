package tui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

var testNow = time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC)

var testEnv = map[string]string{"NO_COLOR": "1"}

// utf8Env draws the Unicode tier without color.
var utf8Env = map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}

// envOf is the test environment. The icon tier defaults to unicode so the assertions keep the glyphs they were
// written against; a test that wants the nerd or ASCII tier sets LOUPE_ICONS itself.
func envOf(m map[string]string) func(string) string {
	return func(k string) string {
		if v, ok := m[k]; ok {
			return v
		}
		if k == style.IconsEnv {
			return "unicode"
		}
		return ""
	}
}

// newFixture writes a run with f-001 (blocking issue at multi.txt:3), f-002 (suggestion at multi.txt:21) and f-003
// (a general question) over the multi-hunk diff.
func newFixture(t *testing.T) string {
	t.Helper()
	return writePRRun(t, multiHunkDiff(t), []draft.FindingInput{
		{Title: "Title one", Body: "Body one explains the rename.", Location: &draft.LocationInput{Path: "multi.txt", Line: 3}, Label: "issue", Blocking: true, Confidence: "high"},
		{Title: "Title two", Body: "Body two suggests a helper.", Location: &draft.LocationInput{Path: "multi.txt", Line: 21}, Label: "suggestion", SuggestedFix: "extract insertAfter"},
		{Title: "Title three", Body: "Body three asks about tests.", General: true, Label: "question"},
	}, func(d *draft.Draft) {
		d.Summary = "Two issues to look at."
		d.Version = 2
	})
}

// multiHunkDiff is testdata/diffs/multi-hunk.diff: multi.txt changed in three hunks.
func multiHunkDiff(t *testing.T) []byte {
	t.Helper()
	diffBytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "diffs", "multi-hunk.diff"))
	if err != nil {
		t.Fatal(err)
	}
	return diffBytes
}

// writePRRun writes round 1 of acme/widgets#42 over diffBytes with inputs filed by the agent, applies edit to the
// draft when it is set, and returns the run's directory.
func writePRRun(t *testing.T, diffBytes []byte, inputs []draft.FindingInput, edit func(*draft.Draft)) string {
	t.Helper()
	dir := run.RunDir(t.TempDir(), "acme", "widgets", 42, 1)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	parsed, err := diff.Parse(diffBytes)
	if err != nil {
		t.Fatal(err)
	}
	d := draft.NewEmpty()
	if _, err := draft.Add(d, inputs, parsed, draft.Rules{}, draft.ByAgent, testNow); err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(d)
	}
	target := run.Target{Schema: run.TargetSchema, Owner: "acme", Repo: "widgets", Number: 42, Round: 1, Title: "Add widgets", DiffSHA256: run.DiffSHA256(diffBytes)}
	for name, v := range map[string]any{"target.json": target, "draft.json": d} {
		if err := run.WriteJSONAtomic(filepath.Join(dir, name), v); err != nil {
			t.Fatal(err)
		}
	}
	if err := run.WriteFileAtomic(filepath.Join(dir, "pr.diff"), diffBytes); err != nil {
		t.Fatal(err)
	}
	return dir
}

// emptyFixture is a round with no findings at all.
func emptyFixture(t *testing.T) string {
	t.Helper()
	dir := newFixture(t)
	d := loadDraft(t, dir)
	d.Findings, d.Summary = []draft.Finding{}, ""
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	return dir
}

// mixedFixture is newFixture with f-001 accepted although blocking, f-002 sent back with a reply and a second note,
// and f-003 excluded and then withdrawn by the agent, so its exclusion is current on an unincluded finding.
func mixedFixture(t *testing.T) string {
	t.Helper()
	dir := newFixture(t)
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		if _, err := draft.Accept(d, "f-001", "", testNow); err != nil {
			return err
		}
		n, err := draft.SendBack(d, "f-002", "Why?", testNow)
		if err != nil {
			return err
		}
		if _, err := draft.AddReply(d, n.ID, "Because.", draft.ByAgent, testNow); err != nil {
			return err
		}
		if _, err := draft.SendBack(d, "f-002", "And this?", testNow); err != nil {
			return err
		}
		_, err = draft.Exclude(d, "f-003", testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[2].Included = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func loadDraft(t *testing.T, dir string) *draft.Draft {
	t.Helper()
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// outside writes the draft the way another loupe process does.
func outside(t *testing.T, dir string, fn func(*draft.Draft) error) {
	t.Helper()
	if _, err := draft.Mutate(dir, "agent", nil, envOf(nil), fn); err != nil {
		t.Fatal(err)
	}
}

// writeDraftAsIs writes d straight to draft.json, as an outside writer that bypasses loupe would, so a test can drop a
// finding no loupe command removes.
func writeDraftAsIs(t *testing.T, dir string, d *draft.Draft) {
	t.Helper()
	d.Version++
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
}

// corruptDraft writes a draft.json that no longer decodes.
func corruptDraft(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "draft.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// breakLock puts a directory where the run's lock file goes, so every write to the draft fails with an error that is
// not a refusal.
func breakLock(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, ".lock")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

// wantLockFailure checks that the review ended on the lock error breakLock causes.
func wantLockFailure(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if _, isRefusal := refusal.As(m.err); m.err == nil || isRefusal || !strings.Contains(m.err.Error(), "open lock") || !isQuit(cmd) {
		t.Fatalf("err %v, quit %v", m.err, isQuit(cmd))
	}
}

func modelOf(t *testing.T, dir string, env map[string]string, width, height int) *Model {
	t.Helper()
	m, err := New(Config{Dir: dir, Getenv: envOf(env), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func newModel(t *testing.T, dir string) *Model {
	t.Helper()
	return modelOf(t, dir, testEnv, 100, 40)
}

func mustOpenFinding(t *testing.T, m *Model, id string) {
	t.Helper()
	if err := m.openFinding(id); err != nil {
		t.Fatal(err)
	}
}

func mustOpen(m *Model, id string) {
	if err := m.openFinding(id); err != nil {
		panic(err)
	}
}

func update(t *testing.T, m *Model, msg tea.Msg) {
	t.Helper()
	m.Update(msg)
	if m.Err() != nil {
		t.Fatalf("model error: %v", m.Err())
	}
}

func press(m *Model, keys ...string) {
	for _, k := range keys {
		m.Update(keyOf(k))
	}
}

// pressKeys is press that fails the test on a model error after any key.
func pressKeys(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	for _, k := range keys {
		update(t, m, keyOf(k))
	}
}

func keyOf(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+r":
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	return runes(s)
}

func runes(k string) tea.KeyPressMsg {
	code := rune(0)
	if rs := []rune(k); len(rs) == 1 {
		code = rs[0]
	}
	return tea.KeyPressMsg{Code: code, Text: k}
}

func arrow(t rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: t} }

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// drain runs cmd and every command a batch or sequence holds, and returns the messages they produce.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, drain(c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

// bodyTop is the first row under the header and the blank row that follows it in a window of height rows.
func bodyTop(height int) int { return 1 + style.ReviewLayout{Height: height}.Gap() }

// sgrOf is the opening SGR sequence of a styled one-cell string, so a test can look for a role's color without
// pinning its value.
func sgrOf(t *testing.T, styled string) string {
	t.Helper()
	sgr, _, ok := strings.Cut(styled, "x")
	if !ok || !strings.HasPrefix(sgr, "\x1b[") {
		t.Fatalf("not a styled cell: %q", styled)
	}
	return sgr
}

var (
	ctrlS    = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	tabKey   = tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
	escKey   = tea.KeyPressMsg{Code: tea.KeyEsc}
)

// readyFixture accepts every finding, so f-001 is an accepted blocking finding, and records viewer and author.
func readyFixture(t *testing.T, viewer, author string) string {
	t.Helper()
	dir := newFixture(t)
	target, err := run.LoadTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	target.Viewer, target.Author, target.URL, target.HeadSHA = viewer, author, "https://github.com/acme/widgets/pull/42", "1111111111111111111111111111111111111111"
	if err := run.WriteJSONAtomic(filepath.Join(dir, "target.json"), target); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		for _, f := range d.Findings {
			if _, err := draft.Accept(d, f.ID, "", testNow); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// footerText is the detail footer as plain text.
func footerText(m *Model) string { return ansi.Strip(strings.Join(m.detailFooter(), " ")) }
