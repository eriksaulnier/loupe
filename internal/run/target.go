package run

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const TargetSchema = 1

// Target is written once by capture and never changed.
type Target struct {
	Schema int    `json:"schema"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Viewer string `json:"viewer"`
	// BaseSHA and HeadSHA come from the API and were verified against the fetched refs.
	BaseSHA string `json:"baseSha"`
	HeadSHA string `json:"headSha"`
	// Round is 1-based, one higher than any existing round for the pull request.
	Round int `json:"round"`
	// PreviousRound is Round-1 when any earlier round exists, published or not; lineage only.
	PreviousRound int       `json:"previousRound,omitempty"`
	CapturedAt    time.Time `json:"capturedAt"`
	ClonePath     string    `json:"clonePath"`
	// BaseRef and HeadRef are refs/loupe/<owner>/<repo>/<number>/<round>/{base,head}.
	BaseRef    string `json:"baseRef"`
	HeadRef    string `json:"headRef"`
	DiffSHA256 string `json:"diffSha256"`
}

func LoadTarget(dir string) (Target, error) {
	path := filepath.Join(dir, "target.json")
	var t Target
	if err := ReadJSON(path, &t); err != nil {
		return Target{}, err
	}
	if t.Schema != TargetSchema {
		return Target{}, recordRefusal(path, fmt.Errorf("schema is %d, expected %d", t.Schema, TargetSchema))
	}
	return t, nil
}

// CreateRun takes the draft as bytes because run must not import draft. The run appears complete or not at all:
// files are written to a sibling temp directory that is renamed into place.
func CreateRun(dir string, target Target, diff []byte, draftJSON []byte) (err error) {
	if _, statErr := os.Lstat(dir); !errors.Is(statErr, fs.ErrNotExist) {
		return refusal.New(refusal.Internal, fmt.Sprintf("run directory %s already exists", dir), "file an issue")
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", parent, err)
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp run directory in %s: %w", parent, err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(tmp)
		}
	}()
	if err = WriteJSONAtomic(filepath.Join(tmp, "target.json"), target); err != nil {
		return err
	}
	if err = WriteFileAtomic(filepath.Join(tmp, "pr.diff"), diff); err != nil {
		return err
	}
	if err = WriteFileAtomic(filepath.Join(tmp, "draft.json"), draftJSON); err != nil {
		return err
	}
	if err = os.Rename(tmp, dir); err != nil {
		if _, statErr := os.Lstat(dir); statErr == nil {
			return refusal.New(refusal.Internal, fmt.Sprintf("run directory %s already exists", dir), "file an issue")
		}
		return fmt.Errorf("rename %s to %s: %w", tmp, dir, err)
	}
	return syncDir(parent)
}
