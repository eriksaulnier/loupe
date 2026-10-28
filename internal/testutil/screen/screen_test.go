package screen

import (
	"fmt"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

// keyLog shows every key it has read, one per row, and ends on q.
type keyLog struct{ keys []string }

func (k *keyLog) Init() tea.Cmd { return nil }

func (k *keyLog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if key.String() == "q" {
			return k, tea.Quit
		}
		k.keys = append(k.keys, key.String())
	}
	return k, nil
}

func (k *keyLog) View() tea.View {
	rows := make([]string, len(k.keys))
	for i, key := range k.keys {
		rows[i] = fmt.Sprintf("key %d: [%s]", i+1, key)
	}
	v := tea.NewView(strings.Join(rows, "\n"))
	v.AltScreen = true
	return v
}

func run(stdin io.Reader, stdout, _ io.Writer) int {
	if _, err := tea.NewProgram(&keyLog{}, tea.WithInput(stdin), tea.WithOutput(stdout), tea.WithWindowSize(100, 40), tea.WithColorProfile(colorprofile.ASCII), tea.WithEnvironment([]string{"TERM=xterm-256color"})).Run(); err != nil {
		return 1
	}
	return 0
}

// Every key Press sends reaches the program as that key, esc included: written on its own, it does not
// join the key after it into an alt sequence.
func TestPressSendsEachKeyAsItself(t *testing.T) {
	s := Start(t, 100, 40, run)
	want := []string{"a", "enter", "esc", "x", "tab", "up", "down", "left", "right", "pgup", "pgdown", "backspace", "ctrl+c", "space"}
	for i, key := range want {
		name := key
		if key == " " {
			name = "space"
		}
		s.Press(name)
		s.WaitFor(fmt.Sprintf("key %d: [%s]", i+1, key))
	}
	// Bubble Tea v2 emits one key message per character.
	s.Type("hello there")
	want = append(want, "h", "e", "l", "l", "o", "space", "t", "h", "e", "r", "e")
	s.WaitFor(fmt.Sprintf("key %d: [e]", len(want)))

	frame := s.View()
	for i, key := range want {
		if row := fmt.Sprintf("key %d: [%s]", i+1, key); !strings.Contains(frame, row) {
			t.Errorf("the frame lacks %q:\n%s", row, frame)
		}
	}
	s.Press("q")
	if _, _, exit := s.Wait(); exit != 0 {
		t.Fatalf("exit %d", exit)
	}
}

func TestViewReplaysCellUpdates(t *testing.T) {
	s := &Screen{changed: make(chan struct{})}
	s.initTerminal(100, 40)
	t.Cleanup(func() { _ = s.terminal.Close() })
	w := stream{s, &s.stdout}
	for _, part := range []string{"\x1b[?1049h\x1b[Halpha beta", "\x1b[1;7H", "BETA", "\x1b[2;1Hwide: 界é", "\x1b[1;6H\x1b[K"} {
		if _, err := io.WriteString(w, part); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.TrimSpace(s.View()); got != "alpha\nwide: 界é" {
		t.Fatalf("screen: %q", got)
	}
}

func TestViewWaitsForSynchronizedOutput(t *testing.T) {
	s := &Screen{changed: make(chan struct{})}
	s.initTerminal(20, 4)
	t.Cleanup(func() { _ = s.terminal.Close() })
	w := stream{s, &s.stdout}
	for _, part := range []string{"old", "\x1b[?2026h", "\x1b[Hnew", "\x1b[?2026"} {
		if _, err := io.WriteString(w, part); err != nil {
			t.Fatal(err)
		}
		if got := s.View(); strings.TrimSpace(got) != "old" {
			t.Fatalf("partial frame: %q", got)
		}
	}
	if _, err := io.WriteString(w, "l"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(s.View()); got != "new" {
		t.Fatalf("completed frame: %q", got)
	}
	generation := s.generation
	if _, err := io.WriteString(w, "\x1b[?25l"); err != nil {
		t.Fatal(err)
	}
	if s.generation != generation {
		t.Fatal("cursor visibility produced a fresh content frame")
	}
}

func TestViewKeepsAlternateScreenAndSplitUnicode(t *testing.T) {
	s := &Screen{changed: make(chan struct{})}
	s.initTerminal(12, 3)
	t.Cleanup(func() { _ = s.terminal.Close() })
	w := stream{s, &s.stdout}
	for _, part := range []string{"main", "\x1b[?1049h", "界é"[:2], "界é"[2:], "\x1b[1;4H!"} {
		if _, err := io.WriteString(w, part); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.TrimSpace(s.View()); got != "界é!" {
		t.Fatalf("Unicode cells: %q", got)
	}
	if _, err := io.WriteString(w, "\x1b[?1049l"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(s.View()); got != "main" {
		t.Fatalf("primary screen: %q", got)
	}
	s.terminal.Resize(6, 2)
	if _, err := io.WriteString(w, "\x1b[2;1Hresize"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(s.View()); got != "main\nresize" {
		t.Fatalf("resized screen: %q", got)
	}
}
