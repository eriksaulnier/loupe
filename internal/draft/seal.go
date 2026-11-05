package draft

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// ReturnFile seals a local run once the human returns its decisions to the agent. After it exists, Mutate refuses
// every change, and loupe review and loupe handoff refuse the run.
const ReturnFile = "return.json"

// ReturnSchema versions return.json (docs/versioning.md).
const ReturnSchema = 1

// Return is the record Seal writes.
type Return struct {
	Schema       int       `json:"schema"`
	ReturnedAt   time.Time `json:"returnedAt"`
	DraftVersion int       `json:"draftVersion"`
	// Note is the human's closing note on the work as a whole, absent when they wrote none.
	Note string `json:"note,omitempty"`
}

// MaxReturnNoteRunes caps the closing note.
const MaxReturnNoteRunes = 4096

const returnedFix = "loupe feedback --run <ref> to read the decisions, or loupe capture --changed|--files|--no-files to start a new run"

// Seal returns a local run's decisions with the human's closing note, which may be empty. It checks the note first,
// so a refused note writes nothing, then under the draft lock refuses a draft that moved past expectVersion, is not
// ready, or holds no finding, and otherwise writes return.json.
func Seal(dir string, expectVersion int, note string, getenv func(string) string, now time.Time) error {
	if err := ValidateReturnNote(note); err != nil {
		return err
	}
	return underLock(dir, "review", getenv, func(d *Draft) error {
		if err := refuseReturned(dir); err != nil {
			return err
		}
		if d.Version != expectVersion {
			return refusal.New(refusal.Changed, fmt.Sprintf("the draft moved to version %d while you decided at version %d", d.Version, expectVersion),
				"read the changes in loupe review, then return again")
		}
		if err := ReadinessRefusal(d); err != nil {
			return err
		}
		if len(d.Findings) == 0 {
			return refusal.New(refusal.Empty, "the draft holds no finding, so there is nothing to return", "ask the agent to file findings with loupe add")
		}
		return run.WriteJSONAtomic(filepath.Join(dir, ReturnFile), Return{Schema: ReturnSchema, ReturnedAt: now.UTC(), DraftVersion: d.Version, Note: note})
	})
}

// ValidateReturnNote refuses a closing note over MaxReturnNoteRunes, outside the Markdown allowlist, or citing a
// finding id. An empty note passes.
func ValidateReturnNote(note string) error {
	const fix = "edit the note, or clear it to return without one"
	if note == "" {
		return nil
	}
	if n := utf8.RuneCountInString(note); n > MaxReturnNoteRunes {
		return refusal.New(refusal.Input, fmt.Sprintf("the closing note is %d characters; at most %d are allowed", n, MaxReturnNoteRunes), fix)
	}
	if err := markdown.Check(note, markdown.ReturnNote, fix); err != nil {
		return err
	}
	return markdown.FindingIDs(note, markdown.ReturnNote, "return again")
}

func refuseReturned(dir string) error {
	returned, err := run.HasReturn(dir)
	if err != nil || !returned {
		return err
	}
	return refusal.New(refusal.Returned, "the run was returned, so its draft can no longer change", returnedFix)
}

// ReadinessRefusal refuses a draft with a pending finding or an open note, which neither publish nor return takes.
func ReadinessRefusal(d *Draft) error {
	r := ReadinessOf(d)
	if r.Ready {
		return nil
	}
	var parts []string
	if len(r.Pending) > 0 {
		parts = append(parts, "pending findings "+strings.Join(r.Pending, ", "))
	}
	if len(r.OpenNotes) > 0 {
		parts = append(parts, "open notes "+strings.Join(r.OpenNotes, ", "))
	}
	return refusal.New(refusal.NotReady, "the draft is not ready: "+strings.Join(parts, "; "), "loupe review")
}

// LoadReturn reads return.json; ok is false while the run has not been returned.
func LoadReturn(dir string) (r Return, ok bool, err error) {
	returned, err := run.HasReturn(dir)
	if err != nil || !returned {
		return Return{}, false, err
	}
	if err := run.ReadJSON(filepath.Join(dir, ReturnFile), &r, ReturnSchema); err != nil {
		return Return{}, false, err
	}
	return r, true, nil
}
