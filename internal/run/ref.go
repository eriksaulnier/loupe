package run

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// Ref names a run. Round 0 means the newest round of the pull request.
type Ref struct {
	Owner  string
	Repo   string
	Number int
	Round  int
}

var refPattern = regexp.MustCompile(`^([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)#([1-9][0-9]*)(?:@([1-9][0-9]*))?$`)

func ParseRef(s string) (Ref, error) {
	m := refPattern.FindStringSubmatch(s)
	if m == nil {
		return Ref{}, invalidRef(s)
	}
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

func invalidRef(s string) error {
	return refusal.New(refusal.Usage, fmt.Sprintf("invalid run reference %q", s), "use owner/repo#123 or owner/repo#123@2")
}

func (r Ref) String() string {
	s := fmt.Sprintf("%s/%s#%d", r.Owner, r.Repo, r.Number)
	if r.Round > 0 {
		s += fmt.Sprintf("@%d", r.Round)
	}
	return s
}
