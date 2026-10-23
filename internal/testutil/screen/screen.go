// Package screen drives loupe's full-screen programs the way a human at a terminal does: keys go in through a pipe,
// and each step waits for the screen to show its result rather than for time to pass, so a test never races the
// program, however slowly a step such as publishing runs.
package screen

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// guard only turns a hung test into a failure that shows the screen; nothing waits for it to pass.
const guard = 10 * time.Second

// keys are the named keys Press sends, as a terminal encodes them. Each is written on its own, which is what keeps
// esc from joining the key after it into one alt-key sequence.
var keys = map[string]string{
	"enter": "\r", "tab": "\t", "esc": "\x1b", "backspace": "\x7f", "space": " ", "ctrl+c": "\x03",
	"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D", "pgup": "\x1b[5~", "pgdown": "\x1b[6~",
}

type Screen struct {
	t  testing.TB
	in *io.PipeWriter
	mu sync.Mutex
	// changed is closed and replaced on every write and when the program ends, so a waiter wakes on news.
	changed        chan struct{}
	drawn          bytes.Buffer
	stdout, stderr bytes.Buffer
	// seen is how much of drawn earlier waits matched, so each wait looks only at what was drawn since.
	seen  int
	ended bool
	exit  int
}

// Start runs fn on its own goroutine. fn reads the keys Screen sends from stdin, and everything it writes to stdout or
// stderr is what the screen shows.
func Start(t testing.TB, fn func(stdin io.Reader, stdout, stderr io.Writer) int) *Screen {
	t.Helper()
	pr, pw := io.Pipe()
	s := &Screen{t: t, in: pw, changed: make(chan struct{})}
	go func() {
		exit := fn(pr, stream{s, &s.stdout}, stream{s, &s.stderr})
		// The program no longer reads, so a key sent from here on fails instead of blocking on the pipe.
		_ = pr.CloseWithError(errors.New("the program has ended"))
		s.mu.Lock()
		s.ended, s.exit = true, exit
		s.notify()
		s.mu.Unlock()
	}()
	t.Cleanup(func() { _ = pw.Close() })
	return s
}

type stream struct {
	s   *Screen
	buf *bytes.Buffer
}

func (w stream) Write(p []byte) (int, error) {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	w.buf.Write(p)
	w.s.drawn.Write(p)
	w.s.notify()
	return len(p), nil
}

// notify is called with mu held.
func (s *Screen) notify() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// Type sends text in one write, as fast typing or a paste arrives.
func (s *Screen) Type(text string) {
	s.t.Helper()
	s.send(text, text)
}

// Press sends each named key on its own: a key from the keys table, or a single character.
func (s *Screen) Press(names ...string) {
	s.t.Helper()
	for _, name := range names {
		seq, ok := keys[name]
		if !ok {
			if len([]rune(name)) != 1 {
				s.t.Fatalf("screen: unknown key %q", name)
			}
			seq = name
		}
		s.send(name, seq)
	}
}

func (s *Screen) send(name, seq string) {
	s.t.Helper()
	if _, err := io.WriteString(s.in, seq); err != nil {
		s.t.Fatalf("screen: sending %q: %v\n%s", name, err, s.tail())
	}
}

// WaitFor returns everything drawn since the last wait that returned, once it contains every one of want. It fails
// when the program ends first.
func (s *Screen) WaitFor(want ...string) string {
	s.t.Helper()
	deadline := time.After(guard)
	for {
		s.mu.Lock()
		fresh, changed, ended := s.drawn.Bytes()[s.seen:], s.changed, s.ended
		if containsAll(fresh, want) {
			s.seen = s.drawn.Len()
			s.mu.Unlock()
			return string(fresh)
		}
		s.mu.Unlock()
		if ended {
			s.t.Fatalf("screen: the program ended (exit %d) without showing %q\n%s", s.exit, missing(fresh, want), s.tail())
			return ""
		}
		select {
		case <-changed:
		case <-deadline:
			s.t.Fatalf("screen: %q not shown after %s\n%s", missing(fresh, want), guard, s.tail())
			return ""
		}
	}
}

// View is the frame on screen now, one line per row with trailing blanks dropped. It replays what bubbletea's renderer
// writes: a frame starts at the home position and rewrites each changed row whole from its first column, a row it
// leaves unchanged is skipped with a bare line feed, and a shorter frame clears below its last row. Styles and every
// other control sequence are dropped.
func (s *Screen) View() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var rows []string
	row, fresh := 0, true
	p := ansi.NewParser()
	for rest := s.drawn.String(); rest != ""; {
		seq, _, n, _ := ansi.DecodeSequence(rest, ansi.NormalState, p)
		rest = rest[n:]
		switch {
		case seq == ansi.CursorHomePosition:
			row, fresh = 0, true
		case seq == ansi.EraseScreenBelow:
			rows = rows[:min(len(rows), row+1)]
		case seq == "\n":
			row, fresh = row+1, true
		case seq == "\r", ansi.HasCsiPrefix(seq), ansi.HasOscPrefix(seq), ansi.HasEscPrefix(seq), ansi.HasDcsPrefix(seq):
		default:
			for len(rows) <= row {
				rows = append(rows, "")
			}
			if fresh {
				rows[row], fresh = "", false
			}
			rows[row] += seq
		}
	}
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return strings.Join(rows, "\n")
}

// Running reports whether the program has yet to end.
func (s *Screen) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.ended
}

// Wait returns once the program ends, with everything it wrote and its exit code.
func (s *Screen) Wait() (stdout, stderr string, exit int) {
	s.t.Helper()
	deadline := time.After(guard)
	for {
		s.mu.Lock()
		changed, ended := s.changed, s.ended
		if ended {
			defer s.mu.Unlock()
			return s.stdout.String(), s.stderr.String(), s.exit
		}
		s.mu.Unlock()
		select {
		case <-changed:
		case <-deadline:
			s.t.Fatalf("screen: the program is still running after %s\n%s", guard, s.tail())
		}
	}
}

func containsAll(b []byte, want []string) bool {
	return len(missing(b, want)) == 0
}

func missing(b []byte, want []string) []string {
	var out []string
	for _, w := range want {
		if !bytes.Contains(b, []byte(w)) {
			out = append(out, w)
		}
	}
	return out
}

// tail is the end of what was drawn, which is the screen a failing step left behind.
func (s *Screen) tail() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	text := s.drawn.String()
	if len(text) > 4000 {
		text = text[len(text)-4000:]
	}
	return fmt.Sprintf("screen tail:\n%s", strings.ToValidUTF8(text, "?"))
}
