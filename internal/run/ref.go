package run

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/secondshift-labs/loupe/internal/refusal"
)

// Ref names a run. Round 0 means the newest round of the pull request. Local > 0 means a local run.
type Ref struct {
	Owner  string
	Repo   string
	Number int
	Round  int
	// Local is the local run number (1-based); non-zero for local refs.
	Local int
	// Prefix is the display-only repository prefix recorded in the target; never checked on input.
	Prefix string
}

var refPattern = regexp.MustCompile(`^([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)#([1-9][0-9]*)(?:@([1-9][0-9]*))?$`)

// localPattern accepts [prefix:]local#n where prefix is owner/repo or a simple name.
var localPattern = regexp.MustCompile(`^(?:([A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)?):)?local#([1-9][0-9]*)$`)

func ParseRef(s string) (Ref, error) {
	if m := refPattern.FindStringSubmatch(s); m != nil {
		number, err := strconv.Atoi(m[3])
		if err != nil {
			return Ref{}, invalidRef(s)
		}
		ref := Ref{Owner: m[1], Repo: m[2], Number: number}
		if m[4] != "" {
			if ref.Round, err = strconv.Atoi(m[4]); err != nil {
				return Ref{}, invalidRef(s)
			}
		}
		return ref, nil
	}
	if m := localPattern.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return Ref{}, invalidRef(s)
		}
		return Ref{Prefix: m[1], Local: n}, nil
	}
	return Ref{}, invalidRef(s)
}

func invalidRef(s string) error {
	return refusal.New(refusal.Usage, fmt.Sprintf("invalid run reference %q", s), "use owner/repo#123 or local#<n>")
}

func (r Ref) String() string {
	if r.Local > 0 {
		if r.Prefix != "" {
			return fmt.Sprintf("%s:local#%d", r.Prefix, r.Local)
		}
		return fmt.Sprintf("local#%d", r.Local)
	}
	s := fmt.Sprintf("%s/%s#%d", r.Owner, r.Repo, r.Number)
	if r.Round > 0 {
		s += fmt.Sprintf("@%d", r.Round)
	}
	return s
}
