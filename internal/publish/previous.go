package publish

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
)

// Bump on any field change, so an older loupe refuses the file instead of dropping fields (docs/versioning.md).
const PreviousSchema = 1

// Previous is the round capture read back from GitHub for a run with no earlier local receipt of its publisher's: the
// publisher's newest loupe review and its findings when Found, and otherwise the reason there is none.
type Previous struct {
	Schema    int    `json:"schema"`
	Found     bool   `json:"found"`
	ReviewID  int64  `json:"reviewId,omitempty"`
	ReviewURL string `json:"reviewUrl,omitempty"`
	Round     int    `json:"round,omitempty"`
	// Commit is the round's own head, empty when a sticky body's could not be read back.
	Commit string `json:"commit,omitempty"`
	// PublicationID names the review's current round, which a sticky review's URL alone does not. Empty in a run
	// captured before it was recorded.
	PublicationID string             `json:"publicationId,omitempty"`
	Findings      []EnvelopeFinding  `json:"findings"`
	Assessments   []draft.Assessment `json:"assessments,omitempty"`
	Reason        string             `json:"reason,omitempty"`
}

// newestOwn is the publisher's newest loupe review: the viewer's own, or when viewer is empty, a [bot]'s from the same
// source. An installation token cannot read its own login, so the source capture recorded is what tells one App's
// review from another's.
func newestOwn(reviews []github.Review, viewer, source string) (github.Review, bool) {
	var newest github.Review
	for _, r := range reviews {
		if r.State == "PENDING" || !ownReview(r, viewer, source) {
			continue
		}
		if r.ID > newest.ID {
			newest = r
		}
	}
	return newest, newest.ID != 0
}

func ownReview(r github.Review, viewer, source string) bool {
	return publishedBy(r, viewer) && (viewer != "" || sameSource(r.Body, source))
}

// PreviousReceipt is the earlier local round holding the run's own publisher's newest review, by the rule newestOwn
// applies to the reviews: a data root two publishers share must not hand one of them findings the other
// accepted. It skips unpublished rounds because only a receipt records what the pull request author saw.
func PreviousReceipt(root string, ref run.Ref, viewer, source string) (int, Receipt, error) {
	skipped := ""
	newest, newestReceipt := 0, Receipt{}
	for round := ref.Round - 1; round >= 1; round-- {
		receipt, found, err := LoadReceipt(run.RunDir(root, ref.Owner, ref.Repo, ref.Number, round))
		if err != nil {
			return 0, Receipt{}, err
		}
		if !found {
			continue
		}
		if ownReview(receiptReview(receipt), viewer, source) {
			if newest == 0 || newerReceipt(receipt, newestReceipt) {
				newest, newestReceipt = round, receipt
			}
			continue
		}
		if skipped == "" {
			skipped = fmt.Sprintf("round %d's receipt was published by %s", round, receiptPublisher(receipt))
		}
	}
	if newest != 0 {
		return newest, newestReceipt, nil
	}
	pr := run.Ref{Owner: ref.Owner, Repo: ref.Repo, Number: ref.Number}
	message := fmt.Sprintf("no earlier round of %s was published", pr)
	if skipped != "" {
		message = fmt.Sprintf("no earlier round of %s was published here by %s: %s", pr, publisher(viewer, source), skipped)
	}
	return 0, Receipt{}, refusal.New(refusal.NotFound, message, fmt.Sprintf("loupe show --run %s", ref))
}

// newerReceipt orders two receipts as GitHub recorded them, since rounds captured together can publish out of order:
// by review id, then within one sticky review by its series round. A full tie is left to the caller's downward scan
// of capture rounds, which keeps the higher one.
func newerReceipt(a, b Receipt) bool {
	if a.ReviewID != b.ReviewID {
		return a.ReviewID > b.ReviewID
	}
	ka, _ := render.StickyRounds(a.Envelope.Body)
	kb, _ := render.StickyRounds(b.Envelope.Body)
	return ka > kb
}

// receiptReview is the review a receipt records. A receipt written before Author was recorded predates unattended
// publication, so its viewer is its author.
func receiptReview(r Receipt) github.Review {
	author := r.Author
	if author == "" {
		author = r.Envelope.Viewer
	}
	return github.Review{User: author, Body: r.Envelope.Body}
}

func receiptPublisher(r Receipt) string {
	review := receiptReview(r)
	if !r.Envelope.Unattended() {
		return review.User
	}
	if source := render.MetaSource(review.Body); source != "" {
		return review.User + " with source " + sourceName(source)
	}
	return review.User + " with no source"
}

// ReadPrevious never fails: the previous round is an aid to the reviewer, so a round that cannot be read is a reason
// the run records, not a capture that refuses. It never falls back to an older review, which the author has since
// seen superseded. It takes capture's one listing of the reviews, and listErr when that listing failed.
func ReadPrevious(reviews []github.Review, listErr error, owner, repo string, number int, viewer, source string) Previous {
	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number)
	none := func(reason string) Previous { return Previous{Schema: PreviousSchema, Reason: reason} }
	if listErr != nil {
		return none(fmt.Sprintf("could not list the reviews on %s: %v", prURL, listErr))
	}
	review, ok := newestOwn(reviews, viewer, source)
	if !ok {
		return none(fmt.Sprintf("no earlier loupe review from %s is on %s", publisher(viewer, source), prURL))
	}
	records, assessed, err := render.ReadRecord(review.Body)
	if err != nil {
		return none(fmt.Sprintf("%s cannot be read back: %v", review.HTMLURL, err))
	}
	round := render.MetaRound(review.Body)
	if round < 1 {
		return none(review.HTMLURL + " cannot be read back: its loupe-meta marker carries no round=")
	}
	findings := make([]EnvelopeFinding, 0, len(records))
	for _, r := range records {
		f := EnvelopeFinding{ID: r.ID, Title: r.Title, Body: r.Body, Label: r.Label, Blocking: r.Blocking}
		if r.Location != nil {
			f.Location = &draft.Location{Path: r.Location.Path, Side: r.Location.Side, Line: r.Location.Line, StartLine: r.Location.StartLine}
		}
		findings = append(findings, f)
	}
	return Previous{Schema: PreviousSchema, Found: true, ReviewID: review.ID, ReviewURL: review.HTMLURL,
		Round: round, Commit: roundCommit(review), PublicationID: render.PublicationID(review.Body), Findings: findings,
		Assessments: draftAssessments(assessed)}
}

// roundCommit is the commit the review's current round reviewed. An edited review keeps the commit_id of the round that
// created it, so a sticky body's own round is read from the body. The findings are exact without it, so a body it
// cannot be read from leaves it empty rather than losing the round.
func roundCommit(review github.Review) string {
	if _, sticky := render.StickyRounds(review.Body); !sticky {
		return review.CommitID
	}
	earlier, _, err := render.ReadSticky(review.Body)
	if err != nil {
		return ""
	}
	_, sha := render.PreviousRound(earlier)
	return sha
}

func draftAssessments(as []render.RecordAssessment) []draft.Assessment {
	var out []draft.Assessment
	for _, a := range as {
		f := a.Finding
		ef := draft.EarlierFinding{ID: f.ID, Title: f.Title, Body: f.Body, Label: f.Label, Blocking: f.Blocking,
			FiledIn: draft.FiledIn{Round: f.FiledIn.Round, ReviewURL: f.FiledIn.ReviewURL, Commit: f.FiledIn.Commit}}
		if f.Location != nil {
			ef.Location = &draft.Location{Path: f.Location.Path, Side: f.Location.Side, Line: f.Location.Line, StartLine: f.Location.StartLine}
		}
		out = append(out, draft.Assessment{Ref: a.Ref, Status: a.Status, Finding: ef})
	}
	return out
}

// Earlier is every finding still open before this round: those the previous round assessed as open, in the order it
// recorded them, which keeps the oldest filing first, then the ones it filed. An open finding therefore rides forward
// one round at a time, and a round never reads further back than the round before it.
func Earlier(filed []EnvelopeFinding, assessments []draft.Assessment, filedIn draft.FiledIn) []draft.EarlierFinding {
	out := []draft.EarlierFinding{}
	for _, a := range assessments {
		if a.Status == draft.StatusOpen {
			out = append(out, a.Finding)
		}
	}
	for _, f := range filed {
		out = append(out, draft.EarlierFinding{ID: f.ID, Title: f.Title, Body: f.Body, Location: f.Location, Label: f.Label,
			Blocking: f.Blocking, FiledIn: filedIn})
	}
	return out
}

// publishedBy is the author half of the publisher rule: a loupe review by the viewer, or with an installation token,
// which cannot read its own login, by any [bot].
func publishedBy(r github.Review, viewer string) bool {
	return authorMatches(r.User, Envelope{Viewer: viewer}) && render.IsLoupe(r.Body)
}

// sameSource leaves the version out, so a release keeps reading and editing the same series.
func sameSource(body, source string) bool {
	return sourceName(render.MetaSource(body)) == sourceName(source)
}

func publisher(viewer, source string) string {
	switch {
	case viewer != "":
		return viewer
	case source == "":
		return "a [bot] with no source"
	}
	return "a [bot] with source " + sourceName(source)
}

func EncodePrevious(p Previous) ([]byte, error) {
	p.Schema = PreviousSchema
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", run.PreviousFile, err)
	}
	return append(data, '\n'), nil
}

// LoadPrevious reports found false only when previous.json does not exist, as for a run captured before it did or after
// a local receipt of its publisher's. A damaged file is a record refusal.
func LoadPrevious(dir string) (Previous, bool, error) {
	var p Previous
	found, err := loadRecord(filepath.Join(dir, run.PreviousFile), &p, PreviousSchema, func() string { return previousProblem(p) })
	return p, found, err
}

// previousProblem holds a found round to everything capture writes for one, so a damaged file is refused rather than
// read as a round that published nothing.
func previousProblem(p Previous) string {
	switch {
	case !p.Found && (p.Reason == "" || p.Findings != nil):
		return "it holds neither a round nor only a reason"
	case !p.Found:
		return ""
	case p.ReviewID < 1 || p.ReviewURL == "" || p.Round < 1 || p.Findings == nil:
		return "its round lacks a review id, a review URL, a round number or its findings"
	}
	for i, f := range p.Findings {
		if f.ID == "" || f.Title == "" {
			return fmt.Sprintf("finding %d lacks an id or a title", i+1)
		}
	}
	for i, a := range p.Assessments {
		if (a.Status != draft.StatusOpen && a.Status != draft.StatusAddressed) || a.Finding.ID == "" || a.Finding.Title == "" {
			return fmt.Sprintf("assessment %d lacks a known status, or its finding's id or title", i+1)
		}
	}
	return ""
}
