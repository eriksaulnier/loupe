package run

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/eriksaulnier/loupe/internal/diff"
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
	BaseRef string `json:"baseRef"`
	HeadRef string `json:"headRef"`
	// MergeBaseSHA is the merge base of BaseRef and HeadRef, the commit pr.diff compares the head against.
	MergeBaseSHA string `json:"mergeBaseSha"`
	DiffSHA256   string `json:"diffSha256"`
	// Source names what filed the findings, as name[@version]; empty when capture was given none.
	Source string `json:"source,omitempty"`
}

// SourcePattern keeps a source to one token without > because loupe-meta carries it unescaped inside an HTML comment.
// ValidateSource also refuses --, which RE2 cannot express, for the same reason. LoadTarget applies it to stored runs,
// so tightening it would refuse runs captured under the looser rule.
const SourcePattern = `^[a-z0-9][a-z0-9._-]*(@[0-9][0-9A-Za-z.+-]*)?$`

const maxSourceChars = 64

var sourceRE = regexp.MustCompile(SourcePattern)

// ValidateSource accepts the empty source, which means none.
func ValidateSource(source string) error {
	if source == "" || (sourceRE.MatchString(source) && len(source) <= maxSourceChars && !strings.Contains(source, "--")) {
		return nil
	}
	return refusal.New(refusal.Input,
		fmt.Sprintf("source %q must match %s, contain no --, and be at most %d characters", source, SourcePattern, maxSourceChars),
		"loupe capture <pr-url> --source <name>[@<version>]")
}

func LoadTarget(dir string) (Target, error) {
	path := filepath.Join(dir, "target.json")
	var t Target
	if err := ReadJSON(path, &t); err != nil {
		return Target{}, err
	}
	if t.Schema != TargetSchema {
		return Target{}, RecordRefusal(path, fmt.Errorf("schema is %d, expected %d", t.Schema, TargetSchema))
	}
	if err := ValidateSource(t.Source); err != nil {
		return Target{}, RecordRefusal(path, err)
	}
	return t, nil
}

// DiffSHA256 is the fingerprint target.json records for pr.diff.
func DiffSHA256(diff []byte) string {
	sum := sha256.Sum256(diff)
	return hex.EncodeToString(sum[:])
}

// LoadDiff parses pr.diff only when it still matches the fingerprint capture recorded, so a truncated or edited diff
// cannot silently change which locations are valid or what the review shows.
func LoadDiff(dir string, target Target) (*diff.Diff, error) {
	path := filepath.Join(dir, "pr.diff")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, RecordRefusal(path, err)
	}
	if got := DiffSHA256(data); got != target.DiffSHA256 {
		return nil, RecordRefusal(path, fmt.Errorf("its SHA-256 is %s but target.json records diffSha256 %s", got, target.DiffSHA256))
	}
	parsed, err := diff.Parse(data)
	if err != nil {
		return nil, RecordRefusal(path, err)
	}
	return parsed, nil
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
