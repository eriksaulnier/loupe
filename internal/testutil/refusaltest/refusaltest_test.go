package refusaltest

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/secondshift-labs/loupe/internal/refusal"
)

// recorder stands in for the test Want runs in, so a failure can be seen rather than failing this test.
type recorder struct {
	testing.TB
	failure string
}

func (r *recorder) Helper() {}

func (r *recorder) Fatalf(format string, args ...any) {
	r.failure = fmt.Sprintf(format, args...)
	panic(r)
}

func want(err error, code refusal.Code) (got *refusal.Error, failure string) {
	r := &recorder{}
	defer func() {
		if p := recover(); p != nil && p != r {
			panic(p)
		}
		failure = r.failure
	}()
	return Want(r, err, code), ""
}

func TestWantReturnsTheRefusal(t *testing.T) {
	in := refusal.New(refusal.Record, "m", "f")
	if got, failure := want(fmt.Errorf("wrapped: %w", in), refusal.Record); got != in || failure != "" {
		t.Fatalf("Want = %v, failure %q", got, failure)
	}
}

func TestWantFailsOnAnotherCodeOrAPlainError(t *testing.T) {
	if _, failure := want(refusal.New(refusal.Usage, "bad flag", "f"), refusal.Record); failure != `code "usage", want "record" (message "bad flag")` {
		t.Fatalf("other code: failure %q", failure)
	}
	if _, failure := want(errors.New("boom"), refusal.Record); !strings.HasSuffix(failure, ", want a record refusal") {
		t.Fatalf("plain error: failure %q", failure)
	}
}
