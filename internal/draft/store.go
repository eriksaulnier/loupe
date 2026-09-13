package draft

import (
	"fmt"
	"path/filepath"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

func Load(dir string) (*Draft, error) {
	var d Draft
	if err := run.ReadJSON(filepath.Join(dir, "draft.json"), &d); err != nil {
		return nil, err
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
	if err := fn(d); err != nil {
		return nil, err
	}
	d.Version++
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		return nil, err
	}
	return d, nil
}
