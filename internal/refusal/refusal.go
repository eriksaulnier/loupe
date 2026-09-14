// Package refusal is a leaf so every domain package can return refusals without importing internal/cli.
package refusal

import "errors"

type Code string

// Codes are exactly the error table in specs/001-loupe-v1/contracts/cli.md; agents branch on them.
const (
	Usage     Code = "usage"
	NoRun     Code = "no-run"
	Record    Code = "record"
	Origin    Code = "origin"
	PR        Code = "pr"
	SameHead  Code = "same-head"
	Auth      Code = "auth"
	Input     Code = "input"
	Location  Code = "location"
	Markdown  Code = "markdown"
	Version   Code = "version"
	Count     Code = "count"
	NotFound  Code = "not-found"
	Lock      Code = "lock"
	TTY       Code = "tty"
	HeadMoved Code = "head-moved"
	OwnPR     Code = "own-pr"
	Blocking  Code = "blocking"
	NotReady  Code = "not-ready"
	Empty     Code = "empty"
	Attempt   Code = "attempt"
	Changed   Code = "changed"
	Viewer    Code = "viewer"
	GitHub    Code = "github"
	Internal  Code = "internal"
)

type Error struct {
	Code    Code
	Message string
	Fix     string
	Details map[string]any
}

func (e *Error) Error() string { return e.Message }

func New(code Code, message, fix string) *Error {
	return &Error{Code: code, Message: message, Fix: fix}
}

func As(err error) (*Error, bool) {
	var r *Error
	if errors.As(err, &r) {
		return r, true
	}
	return nil, false
}
