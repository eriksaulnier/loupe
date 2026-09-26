package draft

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// ErrNoChange is returned by a mutation fn that left the draft as it was, so Mutate writes nothing and keeps the
// version; a bumped version would refuse the next decision made against the unchanged draft as stale.
var ErrNoChange = errors.New("draft unchanged")

func Load(dir string) (*Draft, error) {
	path := filepath.Join(dir, "draft.json")
	var d Draft
	if err := run.ReadJSON(path, &d, SchemaVersion); err != nil {
		return nil, err
	}
	// A draft that decodes with a nil collection would make later mutators panic, so it is refused as damaged.
	problem := ""
	switch {
	case d.Findings == nil:
		problem = "findings is missing or null"
	case d.Decisions == nil:
		problem = "decisions is missing or null"
	case d.Notes == nil:
		problem = "notes is missing or null"
	case d.Replies == nil:
		problem = "replies is missing or null"
	}
	if problem != "" {
		return nil, run.RecordRefusal(path, errors.New(problem))
	}
	return &d, nil
}

// Mutate is the only way a draft changes: read, check the expected version and apply fn all under the run lock, so
// concurrent writers serialize and a failed fn leaves the file untouched.
func Mutate(dir, command string, expectVersion *int, getenv func(string) string, fn func(*Draft) error) (*Draft, error) {
	var d *Draft
	err := underLock(dir, command, getenv, func(loaded *Draft) error {
		if expectVersion != nil && *expectVersion != loaded.Version {
			return refusal.New(refusal.Version,
				fmt.Sprintf("draft is at version %d, expected %d", loaded.Version, *expectVersion),
				"re-read with loupe show --json and retry")
		}
		before := len(loaded.Notes)
		if err := fn(loaded); errors.Is(err, ErrNoChange) {
			d = loaded
			return nil
		} else if err != nil {
			return err
		}
		// A note is handed to the agent as it is written, and before the draft, so no reader sees one without the other.
		// An id whose draft write then fails is harmless: a note awaits only while it exists.
		if len(loaded.Notes) > before {
			if _, err := handBack(dir, loaded, loaded.Notes[before:]); err != nil {
				return err
			}
		}
		loaded.Version++
		if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), loaded); err != nil {
			return err
		}
		d = loaded
		return nil
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// underLock loads the draft under the run lock and holds the lock until fn returns.
func underLock(dir, command string, getenv func(string) string, fn func(*Draft) error) (err error) {
	held, err := run.Lock(dir, command, getenv)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := held.Unlock(); unlockErr != nil && err == nil {
			err = unlockErr
		}
	}()
	d, err := Load(dir)
	if err != nil {
		return err
	}
	return fn(d)
}
