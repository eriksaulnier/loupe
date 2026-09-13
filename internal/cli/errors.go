package cli

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const (
	exitOK      = 0
	exitRefusal = 1
	exitUsage   = 2
)

// report writes err per the output contract and returns the exit code. Any error that is not a refusal is a defect.
func report(stdout, stderr io.Writer, jsonMode bool, command, run string, err error) int {
	if err == nil {
		return exitOK
	}
	r, ok := refusal.As(err)
	if !ok {
		r = refusal.New(refusal.Internal, err.Error(), "file an issue")
		_, _ = fmt.Fprintf(stderr, "internal error: %v\n%s", err, debug.Stack())
	}
	if jsonMode {
		if writeErr := writeRefusal(stdout, command, run, r); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "error: write result: %v\n", writeErr)
		}
	} else {
		_, _ = fmt.Fprintf(stderr, "error: %s\nfix: %s\n", r.Message, r.Fix)
	}
	if r.Code == refusal.Usage {
		return exitUsage
	}
	return exitRefusal
}
