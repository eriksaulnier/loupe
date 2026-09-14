package run

import (
	"fmt"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// PreviousPublished skips unpublished rounds because only a receipt records what the pull request author saw.
func PreviousPublished(root string, ref Ref) (round int, dir string, err error) {
	for r := ref.Round - 1; r >= 1; r-- {
		candidate := RunDir(root, ref.Owner, ref.Repo, ref.Number, r)
		published, err := HasReceipt(candidate)
		if err != nil {
			return 0, "", err
		}
		if published {
			return r, candidate, nil
		}
	}
	pr := Ref{Owner: ref.Owner, Repo: ref.Repo, Number: ref.Number}
	return 0, "", refusal.New(refusal.NotFound, fmt.Sprintf("no earlier round of %s was published", pr),
		fmt.Sprintf("loupe show --run %s", ref))
}
