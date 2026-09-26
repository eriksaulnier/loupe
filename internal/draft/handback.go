package draft

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/eriksaulnier/loupe/internal/run"
)

// Bump on any field change, so an older loupe refuses the file instead of dropping fields (docs/versioning.md).
const HandBackSchema = 1

// HandBack is the set of notes handed to the agent: each note as it is written, and any still open and unanswered
// when a review session quits. It lives beside the draft rather than in it so recording one never bumps the draft version.
type HandBack struct {
	Schema int `json:"schema"`
	// Notes is append-only; a note stays listed after it is answered or closed and Awaiting filters it out.
	Notes []string `json:"notes"`
}

func handBackPath(dir string) string { return filepath.Join(dir, "handback.json") }

// LoadHandBack reads the set; a run that never recorded one is empty.
func LoadHandBack(dir string) (*HandBack, error) {
	path := handBackPath(dir)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return &HandBack{Schema: HandBackSchema, Notes: []string{}}, nil
	}
	var h HandBack
	if err := run.ReadJSON(path, &h, HandBackSchema); err != nil {
		return nil, err
	}
	if h.Notes == nil {
		return nil, run.RecordRefusal(path, errors.New("notes is missing or null"))
	}
	return &h, nil
}

// RecordHandBack adds every open, unanswered note to the set under the run lock and returns how many were new. The
// file is written only when something was added, so a review session that handed nothing back leaves no trace.
func RecordHandBack(dir string, getenv func(string) string) (added int, err error) {
	err = underLock(dir, "review", getenv, func(d *Draft) error {
		added, err = handBack(dir, d, d.Notes)
		return err
	})
	if err != nil {
		return 0, err
	}
	return added, nil
}

// handBack adds each of notes that is open, unanswered and not yet listed to the set, and writes the file only when
// something was added. The caller holds the run lock.
func handBack(dir string, d *Draft, notes []Note) (added int, err error) {
	h, err := LoadHandBack(dir)
	if err != nil {
		return 0, err
	}
	for _, n := range notes {
		if n.Status == NoteOpen && !hasReply(d, n.ID) && !slices.Contains(h.Notes, n.ID) {
			h.Notes = append(h.Notes, n.ID)
			added++
		}
	}
	if added == 0 {
		return 0, nil
	}
	return added, run.WriteJSONAtomic(handBackPath(dir), h)
}
