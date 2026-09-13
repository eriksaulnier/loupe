package tui

import (
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/run"
)

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func detailViewOf(t *testing.T, env map[string]string, body string) string {
	t.Helper()
	dir := newFixture(t)
	d := loadDraft(t, dir)
	d.Findings[0].Body = body
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{Dir: dir, Getenv: envOf(env), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	return m.View()
}

func TestDetailViewNeutralizesCharacterReferences(t *testing.T) {
	cases := []struct {
		name, body, visible string
	}{
		{"decimal escape", "&#27;[31mX", `\u001B[31mX`},
		{"hex OSC with BEL", "&#x1b;]52;c;Zm9v&#7;", `\u001B]52;c;Zm9v\u0007`},
		{"right-to-left override", "&#x202E;abc", `\u202Eabc`},
		{"missing semicolon", "&#27[2J", `\u001B[2J`},
	}
	for mode, env := range map[string]map[string]string{"no color": testEnv, "color": {}} {
		for _, c := range cases {
			t.Run(mode+"/"+c.name, func(t *testing.T) {
				view := detailViewOf(t, env, c.body)
				stripped := sgr.ReplaceAllString(view, "")
				if strings.ContainsAny(stripped, "\x1b\x07\u202e") {
					t.Fatalf("view carries a raw control:\n%q", view)
				}
				if !strings.Contains(stripped, c.visible) {
					t.Fatalf("view does not show %q:\n%s", c.visible, stripped)
				}
			})
		}
	}
}

func TestDetailViewKeepsHarmlessReferences(t *testing.T) {
	view := sgr.ReplaceAllString(detailViewOf(t, testEnv, "a &amp; b"), "")
	if !strings.Contains(view, "a & b") {
		t.Fatalf("view does not render a & b:\n%s", view)
	}
}
