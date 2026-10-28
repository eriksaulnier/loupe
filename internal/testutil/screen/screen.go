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
	"github.com/charmbracelet/x/vt"
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
	terminal       *vt.Emulator
	synchronized   bool
	frame          string
	generation     uint64
	seenGeneration uint64
	stdout, stderr bytes.Buffer
	ended          bool
	exit           int
}

// Start runs fn on its own goroutine. fn reads the keys Screen sends from stdin, and everything it writes to stdout or
// stderr is what the screen shows.
func Start(t testing.TB, width, height int, fn func(stdin io.Reader, stdout, stderr io.Writer) int) *Screen {
	t.Helper()
	pr, pw := io.Pipe()
	s := &Screen{t: t, in: pw, changed: make(chan struct{})}
	s.initTerminal(width, height)
	// The emulator answers terminal queries through the same input as keys. It answers while the program's write holds
	// the screen, so the answers wait in a queue, as a tty's input buffer holds them, rather than in a pipe the program
	// may not read until that write returns.
	var replies sync.WaitGroup
	queue := newReplyQueue()
	replies.Go(func() {
		defer queue.close()
		buf := make([]byte, 4096)
		for {
			n, err := s.terminal.Read(buf)
			queue.push(buf[:n])
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
					s.failReply(err)
				}
				return
			}
		}
	})
	replies.Go(func() {
		for reply, ok := queue.pop(); ok; reply, ok = queue.pop() {
			if _, err := pw.Write(reply); err != nil {
				if !errors.Is(err, io.ErrClosedPipe) {
					s.failReply(err)
				}
				return
			}
		}
	})
	go func() {
		exit := fn(pr, stream{s, &s.stdout}, stream{s, &s.stderr})
		// The program no longer reads, so a key sent from here on fails instead of blocking on the pipe.
		_ = pr.CloseWithError(errors.New("the program has ended"))
		s.mu.Lock()
		s.ended, s.exit = true, exit
		s.notify()
		s.mu.Unlock()
	}()
	t.Cleanup(func() {
		_ = pw.Close()
		// Close the response pipe before Emulator.Close touches its unsynchronized state.
		_ = s.terminal.InputPipe().(io.Closer).Close()
		replies.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		_ = s.terminal.Close()
	})
	return s
}

// failReply records a terminal answer that could not be passed on, unless the program has already ended.
func (s *Screen) failReply(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ended {
		fmt.Fprintf(&s.stderr, "terminal response: %v", err)
	}
}

// replyQueue holds the emulator's answers until the program reads them. It never blocks the emulator.
type replyQueue struct {
	mu     sync.Mutex
	ready  chan struct{}
	queued [][]byte
	closed bool
}

func newReplyQueue() *replyQueue { return &replyQueue{ready: make(chan struct{}, 1)} }

func (q *replyQueue) push(p []byte) {
	if len(p) == 0 {
		return
	}
	q.mu.Lock()
	q.queued = append(q.queued, bytes.Clone(p))
	q.mu.Unlock()
	q.wake()
}

func (q *replyQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.wake()
}

func (q *replyQueue) wake() {
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

// pop waits for the next answer. It reports false once the queue is closed and empty.
func (q *replyQueue) pop() ([]byte, bool) {
	for {
		q.mu.Lock()
		if len(q.queued) > 0 {
			p := q.queued[0]
			q.queued = q.queued[1:]
			q.mu.Unlock()
			return p, true
		}
		closed := q.closed
		q.mu.Unlock()
		if closed {
			return nil, false
		}
		<-q.ready
	}
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
	if _, err := w.s.terminal.Write(p); err != nil {
		return 0, err
	}
	if !w.s.synchronized {
		if frame := w.s.terminal.String(); frame != w.s.frame {
			w.s.frame = frame
			w.s.generation++
		}
	}
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

// WaitFor returns a fresh completed frame that contains every requested string. It fails if the program ends first.
func (s *Screen) WaitFor(want ...string) string {
	s.t.Helper()
	deadline := time.After(guard)
	for {
		s.mu.Lock()
		fresh, changed, ended := []byte(s.frame), s.changed, s.ended
		if s.generation > s.seenGeneration && containsAll(fresh, want) {
			s.seenGeneration = s.generation
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

func (s *Screen) initTerminal(width, height int) {
	s.terminal = vt.NewEmulator(width, height)
	_, _ = s.terminal.WriteString(ansi.SetModeLineFeedNewLine)
	s.terminal.SetCallbacks(vt.Callbacks{
		EnableMode: func(mode ansi.Mode) {
			if mode == ansi.ModeSynchronizedOutput {
				s.synchronized = true
			}
		},
		DisableMode: func(mode ansi.Mode) {
			if mode == ansi.ModeSynchronizedOutput {
				s.synchronized = false
			}
		},
	})
}

// View returns the last completed frame with terminal controls interpreted.
func (s *Screen) View() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frame
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
