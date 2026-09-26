// Package draft holds the review draft: findings, human decisions, send-back notes and replies.
package draft

import (
	"fmt"
	"time"
)

// Bump on any field change, so an older loupe refuses the file instead of dropping fields (docs/versioning.md).
const SchemaVersion = 1

const (
	SideRight = "RIGHT"
	SideLeft  = "LEFT"
)

const (
	DecisionAccepted = "accepted"
	DecisionExcluded = "excluded"
)

const (
	NoteOpen      = "open"
	NoteResolved  = "resolved"
	NoteDismissed = "dismissed"
)

const (
	ByAgent = "agent"
	ByHuman = "human"
)

type Draft struct {
	// Schema is SchemaVersion.
	Schema int `json:"schema"`
	// Version starts at 0 on capture; every mutation increments by one.
	Version int    `json:"version"`
	Summary string `json:"summary"`
	// Findings is append-only so ids are never reused.
	Findings []Finding `json:"findings"`
	// Decisions is keyed by finding id, at most one per finding.
	Decisions map[string]Decision `json:"decisions"`
	Notes     []Note              `json:"notes"`
	Replies   []Reply             `json:"replies"`
}

type Finding struct {
	// ID is f-001, sequential within the run, never reused.
	ID string `json:"id"`
	// Rev starts at 1; increments on any change to a publishable field or to Included.
	Rev   int    `json:"rev"`
	Title string `json:"title"`
	Body  string `json:"body"`
	// Exactly one of Location or General is set.
	Location *Location `json:"location,omitempty"`
	General  bool      `json:"general"`
	// Label is issue, suggestion, question or any other word kept verbatim; empty means none.
	Label    string `json:"label,omitempty"`
	Blocking bool   `json:"blocking"`
	// Confidence is high, medium or low when present.
	Confidence string `json:"confidence,omitempty"`
	// Severity is critical, major, minor or trivial when set through add or edit. A run captured before that rule may
	// hold free text, which still renders and is refused only once an edit changes it.
	Severity string `json:"severity,omitempty"`
	// Verified is reproduced or plausible when present.
	Verified     string   `json:"verified,omitempty"`
	Impact       string   `json:"impact,omitempty"`
	References   []string `json:"references,omitempty"`
	SuggestedFix string   `json:"suggestedFix,omitempty"`
	// By is ByAgent or ByHuman; audit only, it does not affect readiness.
	By string `json:"by"`
	// Included is never settable from JSON input; only edit --exclude and --include change it.
	Included  bool           `json:"included"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	History   []HistoryEntry `json:"history"`
}

type Location struct {
	Path string `json:"path"`
	// Side is SideRight (the new file, and the default) or SideLeft (the old file).
	Side      string `json:"side"`
	Line      int    `json:"line"`
	StartLine int    `json:"startLine,omitempty"`
}

type Decision struct {
	FindingID string `json:"findingId"`
	// Decision is DecisionAccepted or DecisionExcluded.
	Decision string `json:"decision"`
	// FindingRev is the finding's rev when decided; the decision is current only while they are equal.
	FindingRev int       `json:"findingRev"`
	At         time.Time `json:"at"`
}

type Note struct {
	// ID is n-001, sequential.
	ID        string    `json:"id"`
	FindingID string    `json:"findingId"`
	Body      string    `json:"body"`
	At        time.Time `json:"at"`
	// Status is NoteOpen, NoteResolved or NoteDismissed.
	Status   string     `json:"status"`
	ClosedAt *time.Time `json:"closedAt,omitempty"`
}

type Reply struct {
	// ID is r-001, sequential.
	ID     string `json:"id"`
	NoteID string `json:"noteId"`
	Body   string `json:"body"`
	// By is ByAgent or ByHuman.
	By string    `json:"by"`
	At time.Time `json:"at"`
}

type HistoryEntry struct {
	At time.Time `json:"at"`
	By string    `json:"by"`
	// Changed maps each changed field to its previous value.
	Changed map[string]any `json:"changed"`
}

func NewEmpty() *Draft {
	return &Draft{
		Schema:    SchemaVersion,
		Findings:  []Finding{},
		Decisions: map[string]Decision{},
		Notes:     []Note{},
		Replies:   []Reply{},
	}
}

func NextFindingID(d *Draft) string { return nextID("f", len(d.Findings)) }

func NextNoteID(d *Draft) string { return nextID("n", len(d.Notes)) }

func NextReplyID(d *Draft) string { return nextID("r", len(d.Replies)) }

// nextID derives from collection length, which is safe only because collections are append-only.
func nextID(prefix string, n int) string {
	return fmt.Sprintf("%s-%03d", prefix, n+1)
}
