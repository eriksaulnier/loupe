package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/style"
)

// Counts come from the domain derivation: accepted and excluded are decided, a current exclusion on an unincluded
// finding still counts, an open note keeps its finding pending however many replies it has, and blocking counts an
// accepted blocking finding (FR-003, FR-004, FR-006).
func TestHeaderAndBorderCountsFollowTheDerivation(t *testing.T) {
	m := modelOf(t, mixedFixture(t), utf8Env, 100, 30)
	if got := draft.Dispositions(m.draft)["f-003"]; got != draft.DispositionExcluded {
		t.Fatalf("the fixture's f-003 is %s, want excluded", got)
	}
	cells, decided, total := progressOf(m.draft, m.order)
	if decided != 2 || total != 3 || decided > total {
		t.Errorf("decided %d of %d, want 2 of 3", decided, total)
	}
	view := m.View().Content
	if !strings.Contains(view, "2/3 decided") || !strings.Contains(view, "1 blocking · 2 open notes · 1 pending") {
		t.Errorf("header or findings border counts:\n%s", view)
	}
	want := map[string]style.ProgressState{"f-001": style.ProgressAccepted, "f-002": style.ProgressNote, "f-003": style.ProgressExcluded}
	for i, f := range m.order {
		if cells[i] != want[f.ID] {
			t.Errorf("%s has cell %v, want %v", f.ID, cells[i], want[f.ID])
		}
	}

	// A stale accept puts the finding back to pending, and the decided count falls by one.
	if _, err := draft.Mutate(m.cfg.Dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Rev++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if _, decided, total := progressOf(m.draft, m.order); decided != 1 || total != 3 {
		t.Errorf("after a stale accept: %d of %d, want 1 of 3", decided, total)
	}
}

// A withdrawn finding keeps its row and its place in the position, but has no cell and is in neither count.
func TestWithdrawnRowsCountInPositionOnly(t *testing.T) {
	dir := withdrawnFixture(t)
	m := modelOf(t, dir, utf8Env, 100, 30)
	_, decided, total := progressOf(m.draft, m.order)
	rows := listRows(m.View().Content)
	if len(rows) != len(m.order) || total != len(m.order)-1 {
		t.Errorf("%d rows, total %d, for %d findings with one withdrawn", len(rows), total, len(m.order))
	}
	if !strings.Contains(m.View().Content, fmt.Sprintf("%d/%d decided", decided, total)) {
		t.Errorf("count:\n%s", m.View().Content)
	}
	mustOpen(m, m.order[0].ID)
	if !strings.Contains(m.View().Content, fmt.Sprintf(" 1 of %d ", len(m.order))) {
		t.Errorf("position:\n%s", m.View().Content)
	}
}

func TestEmptyRoundShowsZeroDecided(t *testing.T) {
	view := modelOf(t, emptyFixture(t), utf8Env, 100, 30).View().Content
	for _, want := range []string{"0/0 decided", "No findings", "No summary"} {
		if !strings.Contains(view, want) {
			t.Errorf("empty round lacks %q:\n%s", want, view)
		}
	}
	header := ansi.Strip(strings.Split(view, "\n")[0])
	before, _, _ := strings.Cut(header, "0/0 decided")
	if !strings.HasSuffix(strings.TrimRight(before, " "), "Add widgets") {
		t.Errorf("an empty round draws something between the title and the count: %q", header)
	}
}

// Above 30 counted findings the header bar is 20 grouped cells beside the exact count.
func TestLargeRoundGroupsTheBar(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	var many []draft.Finding
	for i := range 31 {
		f := m.draft.Findings[1]
		f.ID = fmt.Sprintf("f-%03d", i+1)
		many = append(many, f)
	}
	m.draft.Findings = many
	m.setDraft(m.draft)
	header := strings.Split(m.View().Content, "\n")[0]
	if !strings.Contains(header, strings.Repeat("-", 20)+"  0/31 decided") || strings.Contains(header, strings.Repeat("-", 21)) {
		t.Errorf("31 findings header %q", header)
	}
}

// The header marks the open finding's cell in detail. The list marks none, since its selection already shows the
// place, and a withdrawn finding has no cell to mark (demo feedback).
func TestProgressMarksTheOpenFinding(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	if got := m.currentCell(); got != 0 {
		t.Errorf("the list marks cell %d, want none", got)
	}
	mustOpen(m, "f-002")
	if got := m.currentCell(); got != 2 {
		t.Errorf("detail marks cell %d, want 2", got)
	}
	second := m.reviewHeader()
	mustOpen(m, "f-003")
	if got := m.currentCell(); got != 3 {
		t.Errorf("detail marks cell %d, want 3", got)
	}
	if m.reviewHeader() == second {
		t.Error("opening another finding left the header unchanged")
	}
	w := modelOf(t, withdrawnFixture(t), utf8Env, 100, 30)
	for _, f := range w.order {
		if draft.Dispositions(w.draft)[f.ID] == draft.DispositionWithdrawn {
			mustOpen(w, f.ID)
			if got := w.currentCell(); got != 0 {
				t.Errorf("a withdrawn finding marks cell %d", got)
			}
		}
	}
}

// Under Orca the header leaves room for the pane controls Orca draws over its top-right corner (demo feedback).
func TestHeaderLeavesRoomForOrcaControls(t *testing.T) {
	for _, sc := range []struct {
		env map[string]string
		gap bool
	}{{utf8Env, false}, {map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8", "ORCA_TERMINAL_HANDLE": "term_1"}, true}} {
		m := modelOf(t, newFixture(t), sc.env, 100, 30)
		header := strings.Split(m.View().Content, "\n")[0]
		if got := strings.HasSuffix(header, "decided     "); got != sc.gap {
			t.Errorf("ORCA_TERMINAL_HANDLE=%q: header %q", sc.env["ORCA_TERMINAL_HANDLE"], header)
		}
	}
}

// A blank row separates the header from the body where there is room for it, and gives way first at short heights
// (demo feedback).
func TestHeaderGapGivesWayAtShortHeights(t *testing.T) {
	for _, sc := range []struct {
		width, height int
		gap           bool
	}{{100, 30, true}, {80, 24, true}, {80, 20, true}, {80, 19, false}, {60, 12, false}} {
		m := modelOf(t, newFixture(t), utf8Env, sc.width, sc.height)
		for _, show := range []func(){func() {}, func() { mustOpen(m, "f-001") }} {
			show()
			rows := strings.Split(m.View().Content, "\n")
			if len(rows) != sc.height {
				t.Fatalf("%dx%d is %d rows", sc.width, sc.height, len(rows))
			}
			if got := strings.TrimSpace(rows[1]) == ""; got != sc.gap {
				t.Errorf("%dx%d: gap %v, want %v:\n%s", sc.width, sc.height, got, sc.gap, m.View().Content)
			}
		}
	}
}
