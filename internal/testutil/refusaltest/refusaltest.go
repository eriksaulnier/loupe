// Package refusaltest checks that an error is a loupe refusal of the expected code.
package refusaltest

import (
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// Want fails the test now unless err is a refusal with code, and returns the refusal so the test can check its
// message, fix and details.
func Want(t testing.TB, err error, code refusal.Code) *refusal.Error {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok {
		t.Fatalf("got %#v, want a %s refusal", err, code)
	}
	if r.Code != code {
		t.Fatalf("code %q, want %q (message %q)", r.Code, code, r.Message)
	}
	return r
}
