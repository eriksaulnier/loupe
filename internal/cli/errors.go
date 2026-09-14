package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/style"
)

const (
	exitOK      = 0
	exitRefusal = 1
	exitUsage   = 2
)

// cleanupError marks a failure that left something behind for the user to remove; loupe never removes it itself.
type cleanupError struct {
	err     error
	cleanup []string
}

func (e *cleanupError) Error() string { return e.err.Error() }

func (e *cleanupError) Unwrap() error { return e.err }

// report writes err per the output contract and returns the exit code. Any error that is not a refusal is a defect.
func report(deps Deps, jsonMode bool, command, run string, err error) int {
	if err == nil {
		return exitOK
	}
	stdout, stderr := deps.Stdout, deps.Stderr
	r, ok := refusal.As(err)
	if !ok {
		r = refusal.New(refusal.Internal, err.Error(), "file an issue")
		var pe *panicError
		if errors.As(err, &pe) {
			_, _ = fmt.Fprintf(stderr, "internal error: %v\n%s", err, pe.stack)
		} else {
			_, _ = fmt.Fprintf(stderr, "internal error: %v\n", err)
		}
	}
	var ce *cleanupError
	if errors.As(err, &ce) {
		details := map[string]any{"cleanup": ce.cleanup}
		for k, v := range r.Details {
			details[k] = v
		}
		withCleanup := *r
		withCleanup.Details = details
		r = &withCleanup
	}
	if jsonMode {
		if writeErr := writeRefusal(stdout, command, run, r); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "error: write result: %v\n", writeErr)
		}
	} else {
		writeRefusalText(stderr, deps.errStyle(), deps.width(), r, ce)
	}
	if r.Code == refusal.Usage {
		return exitUsage
	}
	return exitRefusal
}

// refusalIndent is the column the message and the fix start at, past the widest of the three line-start words.
const refusalIndent = "        "

// writeRefusalText prints the refusal on stderr. Without color the shape is the one every earlier version printed,
// `error: <message>` and `fix: <fix>`; color buys the refusal code, wrapping and the commands of the fix on their own
// lines, none of which may move the line-start words the contract fixes.
func writeRefusalText(w io.Writer, s style.Style, width int, r *refusal.Error, ce *cleanupError) {
	if !s.Color {
		_, _ = fmt.Fprintf(w, "error: %s\nfix: %s\n", r.Message, r.Fix)
		if ce != nil {
			_, _ = fmt.Fprintf(w, "cleanup:\n  %s\n", strings.Join(ce.cleanup, "\n  "))
		}
		return
	}
	label := func(st func(...string) string, word string) string {
		return st(word+":") + strings.Repeat(" ", len(refusalIndent)-len(word)-1)
	}
	code := string(r.Code)
	_, _ = fmt.Fprintf(w, "%s%s  %s\n", label(s.Bad.Bold(true).Render, "error"), s.Dim.Render(code),
		hanging(s, r.Message, width, refusalIndent+strings.Repeat(" ", len(code)+2)))
	_, _ = fmt.Fprintf(w, "%s%s\n", label(s.Good.Bold(true).Render, "fix"), fixLines(s, r.Fix, width))
	if ce != nil {
		_, _ = fmt.Fprintf(w, "%s\n", s.Bad.Bold(true).Render("cleanup:"))
		for _, c := range ce.cleanup {
			_, _ = fmt.Fprintf(w, "  %s\n", s.Accent.Render(c))
		}
	}
}

// hanging wraps text under a first line that already sits at the indent column.
func hanging(s style.Style, text string, width int, indent string) string {
	return strings.TrimPrefix(s.Wrap(text, width, indent), indent)
}

// fixLines puts every `loupe …` command of a fix on its own indented line so it can be copied whole; only the prose
// between the commands is wrapped. The first line starts where the caller's label ends.
func fixLines(s style.Style, fix string, width int) string {
	var lines []string
	prose := func(text string) {
		if text = strings.Trim(text, " ,"); text != "" {
			lines = append(lines, s.Wrap(text, width, refusalIndent))
		}
	}
	rest := fix
	for strings.Contains(rest, "loupe ") {
		at := strings.Index(rest, "loupe ")
		prose(rest[:at])
		command := rest[at:]
		if end := commandEnd(command); end >= 0 {
			rest, command = command[end:], command[:end]
		} else {
			rest = ""
		}
		lines = append(lines, refusalIndent+"  "+s.Accent.Render(strings.Trim(command, " ,")))
	}
	prose(rest)
	if len(lines) == 0 {
		return fix
	}
	return strings.TrimPrefix(strings.Join(lines, "\n"), refusalIndent)
}

// commandEnd is where a command inside a fix sentence stops and prose starts again.
func commandEnd(s string) int {
	end := -1
	for _, sep := range []string{", ", " and ", " then "} {
		if at := strings.Index(s, sep); at >= 0 && (end < 0 || at < end) {
			end = at
		}
	}
	return end
}
