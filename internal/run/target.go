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

// Bump on any field change, so an older loupe refuses the file instead of dropping fields (docs/versioning.md).
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
	// Model is the reviewer's model id as the caller names it; empty when capture was given none. Provenance only.
	Model string `json:"model,omitempty"`
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

// ModelPattern admits ids like anthropic/claude-sonnet-5 and, like SourcePattern, keeps > out because loupe-meta
// carries the value unescaped inside an HTML comment. ValidateModel refuses -- for the same reason.
const ModelPattern = `^[a-z0-9][a-z0-9._/:-]*$`

const maxModelChars = 64

var modelRE = regexp.MustCompile(ModelPattern)

// ValidateModel accepts the empty model, which means none.
func ValidateModel(model string) error {
	if model == "" || (modelRE.MatchString(model) && len(model) <= maxModelChars && !strings.Contains(model, "--")) {
		return nil
	}
	return refusal.New(refusal.Input,
		fmt.Sprintf("model %q must match %s, contain no --, and be at most %d characters", model, ModelPattern, maxModelChars),
		"loupe capture <pr-url> --model <id>")
}

func LoadTarget(dir string) (Target, error) {
	path := filepath.Join(dir, "target.json")
	var t Target
	if err := ReadJSON(path, &t, TargetSchema); err != nil {
		return Target{}, err
	}
	if err := ValidateSource(t.Source); err != nil {
		return Target{}, RecordRefusal(path, err)
	}
	if err := ValidateModel(t.Model); err != nil {
		return Target{}, RecordRefusal(path, err)
	}
	return t, nil
}

// DiffSHA256 is the fingerprint target.json records for pr.diff.
func DiffSHA256(diff []byte) string {
	sum := sha256.Sum256(diff)
	return hex.EncodeToString(sum[:])
}

// ReadDiff returns pr.diff only when it still matches the fingerprint capture recorded, so a truncated or edited diff
// cannot silently change which locations are valid, what the review shows, or what a pipeline hands its reviewer.
func ReadDiff(dir string, target Target) ([]byte, error) {
	path := filepath.Join(dir, "pr.diff")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, RecordRefusal(path, err)
	}
	if got := DiffSHA256(data); got != target.DiffSHA256 {
		return nil, RecordRefusal(path, fmt.Errorf("its SHA-256 is %s but target.json records diffSha256 %s", got, target.DiffSHA256))
	}
	return data, nil
}

// LoadDiff parses the diff ReadDiff verified.
func LoadDiff(dir string, target Target) (*diff.Diff, error) {
	data, err := ReadDiff(dir, target)
	if err != nil {
		return nil, err
	}
	parsed, err := diff.Parse(data)
	if err != nil {
		return nil, RecordRefusal(filepath.Join(dir, "pr.diff"), err)
	}
	return parsed, nil
}

// PreviousFile holds the round capture read back from GitHub. See publish.Previous.
const PreviousFile = "previous.json"

// CommentsFile holds the feedback capture read from other reviewers. See publish.Comments.
const CommentsFile = "comments.json"

// CreateRun takes the draft and the optional files, such as PreviousFile and CommentsFile, as bytes because run must
// not import draft or publish. The run appears complete or not at all: files are written to a sibling temp directory
// that is renamed into place.
func CreateRun(dir string, target Target, diff, draftJSON []byte, optional map[string][]byte) (err error) {
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
	target.Schema = TargetSchema
	if err = WriteJSONAtomic(filepath.Join(tmp, "target.json"), target); err != nil {
		return err
	}
	if err = WriteFileAtomic(filepath.Join(tmp, "pr.diff"), diff); err != nil {
		return err
	}
	if err = WriteFileAtomic(filepath.Join(tmp, "draft.json"), draftJSON); err != nil {
		return err
	}
	for name, data := range optional {
		if err = WriteFileAtomic(filepath.Join(tmp, name), data); err != nil {
			return err
		}
	}
	if err = os.Rename(tmp, dir); err != nil {
		if _, statErr := os.Lstat(dir); statErr == nil {
			return refusal.New(refusal.Internal, fmt.Sprintf("run directory %s already exists", dir), "file an issue")
		}
		return fmt.Errorf("rename %s to %s: %w", tmp, dir, err)
	}
	return syncDir(parent)
}
