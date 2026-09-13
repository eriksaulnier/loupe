package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/eriksaulnier/loupe/internal/refusal"
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
func report(stdout, stderr io.Writer, jsonMode bool, command, run string, err error) int {
	if err == nil {
		return exitOK
	}
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
		_, _ = fmt.Fprintf(stderr, "error: %s\nfix: %s\n", r.Message, r.Fix)
		if ce != nil {
			_, _ = fmt.Fprintf(stderr, "cleanup:\n  %s\n", strings.Join(ce.cleanup, "\n  "))
		}
	}
	if r.Code == refusal.Usage {
		return exitUsage
	}
	return exitRefusal
}
