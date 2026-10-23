package screen

import (
	"fmt"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// keyLog shows every key it has read, one per row, and ends on q.
type keyLog struct{ keys []string }

func (k *keyLog) Init() tea.Cmd { return nil }

func (k *keyLog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if key.String() == "q" {
			return k, tea.Quit
		}
		k.keys = append(k.keys, key.String())
	}
	return k, nil
}

func (k *keyLog) View() string {
	rows := make([]string, len(k.keys))
	for i, key := range k.keys {
		rows[i] = fmt.Sprintf("key %d: [%s]", i+1, key)
	}
	return strings.Join(rows, "\n")
}

func run(stdin io.Reader, stdout, _ io.Writer) int {
	if _, err := tea.NewProgram(&keyLog{}, tea.WithInput(stdin), tea.WithOutput(stdout), tea.WithAltScreen()).Run(); err != nil {
		return 1
	}
	return 0
}

// Every key Press sends reaches the program as that key, esc included: written on its own, it does not
// join the key after it into an alt sequence.
func TestPressSendsEachKeyAsItself(t *testing.T) {
	s := Start(t, run)
	want := []string{"a", "enter", "esc", "x", "tab", "up", "down", "left", "right", "pgup", "pgdown", "backspace", "ctrl+c", " "}
	for i, key := range want {
		name := key
		if key == " " {
			name = "space"
		}
		s.Press(name)
		s.WaitFor(fmt.Sprintf("key %d: [%s]", i+1, key))
	}
	// Bubble Tea reads typed text as runs of runes, with each space a key of its own.
	s.Type("hello there")
	s.WaitFor("[hello]", "[ ]", "[there]")

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
