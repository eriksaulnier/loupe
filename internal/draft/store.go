package draft

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// ErrNoChange is returned by a mutation fn that left the draft as it was, so Mutate writes nothing and keeps the version;
// a bumped version would refuse the next decision made against the unchanged draft as stale.
var ErrNoChange = errors.New("draft unchanged")

func Load(dir string) (*Draft, error) {
	path := filepath.Join(dir, "draft.json")
	var d Draft
	if err := run.ReadJSON(path, &d); err != nil {
		return nil, err
	}
	// A draft that decodes with a nil collection would make later mutators panic, so it is refused as damaged.
	problem := ""
	switch {
	case d.Schema != SchemaVersion:
		problem = fmt.Sprintf("schema is %d, expected %d", d.Schema, SchemaVersion)
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
func Mutate(dir, command string, expectVersion *int, getenv func(string) string, fn func(*Draft) error) (d *Draft, err error) {
	held, err := run.Lock(dir, command, getenv)
	if err != nil {
		return nil, err
	}
	defer func() {
		if unlockErr := held.Unlock(); unlockErr != nil && err == nil {
			d, err = nil, unlockErr
		}
	}()
	d, err = Load(dir)
	if err != nil {
		return nil, err
	}
	if expectVersion != nil && *expectVersion != d.Version {
		return nil, refusal.New(refusal.Version,
			fmt.Sprintf("draft is at version %d, expected %d", d.Version, *expectVersion),
			"re-read with loupe show --json and retry")
	}
	if err := fn(d); errors.Is(err, ErrNoChange) {
		return d, nil
	} else if err != nil {
		return nil, err
	}
	d.Version++
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		return nil, err
	}
	return d, nil
}
