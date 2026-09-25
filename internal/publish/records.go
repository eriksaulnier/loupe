// Package publish runs the publication state machine: gates, envelope, confirmation, one review request, receipt.
package publish

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

// Bump on any field change, so an older loupe refuses the file instead of dropping fields (docs/versioning.md).
const RecordSchema = 1

// recordSchema is what this loupe reads and writes, as a var so a test can play the next loupe.
var recordSchema = RecordSchema

const (
	StateInFlight = "in-flight"
	StateUnknown  = "unknown"
)

const (
	attemptFile = "attempt.json"
	receiptFile = "receipt.json"
)

// Envelope is the exact review request that was or may have been sent, plus what a later round needs to read back.
type Envelope struct {
	Target        EnvelopeTarget `json:"target"`
	Viewer        string         `json:"viewer"`
	Action        string         `json:"action"`
	Event         string         `json:"event"`
	CommitID      string         `json:"commitId"`
	DraftVersion  int            `json:"draftVersion"`
	Digest        string         `json:"digest"`
	PublicationID string         `json:"publicationId"`
	Inline        string         `json:"inline"`
	// EditReviewID is the review whose body a sticky round replaces, 0 when the publication creates one.
	EditReviewID int64             `json:"editReviewId,omitempty"`
	Body         string            `json:"body"`
	Comments     []Comment         `json:"comments"`
	Findings     []EnvelopeFinding `json:"findings"`
	// Assessments is the draft's, omitted when there are none so a receipt written before them reads the same.
	Assessments []draft.Assessment `json:"assessments,omitempty"`
}

// Unattended reports whether the envelope was composed by --unattended, which never records a viewer; an attended
// publication always does.
func (e Envelope) Unattended() bool { return e.Viewer == "" }

type EnvelopeTarget struct {
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	HeadSHA string `json:"headSha"`
	Round   int    `json:"round"`
}

type Comment struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine int    `json:"startLine,omitempty"`
	StartSide string `json:"startSide,omitempty"`
	Body      string `json:"body"`
}

type EnvelopeFinding struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Body     string          `json:"body"`
	Location *draft.Location `json:"location"`
	Label    string          `json:"label"`
	Blocking bool            `json:"blocking"`
}

type Attempt struct {
	Schema    int       `json:"schema"`
	State     string    `json:"state"`
	StartedAt time.Time `json:"startedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Envelope  Envelope  `json:"envelope"`
	Confirmed Confirmed `json:"confirmed"`
	LastError string    `json:"lastError,omitempty"`
}

// Confirmed is what the human saw when they pressed y.
type Confirmed struct {
	Version      int               `json:"version"`
	Digest       string            `json:"digest"`
	Dispositions map[string]string `json:"dispositions"`
}

type Receipt struct {
	Schema    int       `json:"schema"`
	ReviewID  int64     `json:"reviewId"`
	ReviewURL string    `json:"reviewUrl"`
	Action    string    `json:"action"`
	PostedAt  time.Time `json:"postedAt"`
	Envelope  Envelope  `json:"envelope"`
	// Author is the login GitHub returned for the review, recorded because an unattended envelope's Viewer is empty.
	Author string `json:"author,omitempty"`
	// Edited is set when the publication replaced an earlier review's body rather than creating a review.
	Edited bool `json:"edited,omitempty"`
}

// LoadAttempt reports found false only when attempt.json does not exist; a damaged file is a record refusal.
func LoadAttempt(dir string) (Attempt, bool, error) {
	var a Attempt
	found, err := loadRecord(filepath.Join(dir, attemptFile), &a, recordSchema, func() string {
		if a.State != StateInFlight && a.State != StateUnknown {
			return fmt.Sprintf("state is %q, expected %q or %q", a.State, StateInFlight, StateUnknown)
		}
		return ""
	})
	return a, found, err
}

func SaveAttempt(dir string, a *Attempt) error {
	a.Schema = recordSchema
	return run.WriteJSONAtomic(filepath.Join(dir, attemptFile), a)
}

func DeleteAttempt(dir string) error {
	path := filepath.Join(dir, attemptFile)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete %s: %w", path, err)
	}
	return nil
}

// LoadReceipt reports found false only when receipt.json does not exist; a damaged file is a record refusal.
func LoadReceipt(dir string) (Receipt, bool, error) {
	var r Receipt
	found, err := loadRecord(filepath.Join(dir, receiptFile), &r, recordSchema, func() string { return "" })
	return r, found, err
}

func SaveReceipt(dir string, r *Receipt) error {
	r.Schema = recordSchema
	return run.WriteJSONAtomic(filepath.Join(dir, receiptFile), r)
}

func loadRecord(path string, v any, schema int, problem func() string) (bool, error) {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err := run.ReadJSON(path, v, schema); err != nil {
		return true, err
	}
	if p := problem(); p != "" {
		return true, run.RecordRefusal(path, errors.New(p))
	}
	return true, nil
}

// newPublicationID is a random (version 4) UUID. crypto/rand.Read never returns an error on supported platforms.
func newPublicationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
