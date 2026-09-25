package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
)

const PreviousSchema = 1

// Previous is the round capture read back from GitHub for a run with no earlier local receipt: the publisher's newest
// loupe review and its findings when Found, and otherwise the reason there is none.
type Previous struct {
	Schema    int               `json:"schema"`
	Found     bool              `json:"found"`
	ReviewID  int64             `json:"reviewId,omitempty"`
	ReviewURL string            `json:"reviewUrl,omitempty"`
	Round     int               `json:"round,omitempty"`
	Findings  []EnvelopeFinding `json:"findings,omitempty"`
	Reason    string            `json:"reason,omitempty"`
}

// newestOwn is the publisher's newest loupe review: the viewer's own, or when viewer is empty, a [bot]'s from the same
// source. An installation token cannot read its own login, so the source capture recorded is what tells one App's
// review from another's. Its version is left out, so a release keeps reading and editing the same series.
func newestOwn(reviews []github.Review, viewer, source string) (github.Review, bool) {
	var newest github.Review
	for _, r := range reviews {
		if r.State == "PENDING" || !authorMatches(r.User, Envelope{Viewer: viewer}) || !render.IsLoupe(r.Body) {
			continue
		}
		if viewer == "" && sourceName(render.MetaSource(r.Body)) != sourceName(source) {
			continue
		}
		if r.ID > newest.ID {
			newest = r
		}
	}
	return newest, newest.ID != 0
}

// ReadPrevious never fails: the previous round is an aid to the reviewer, so a round that cannot be read is a reason
// the run records, not a capture that refuses. It never falls back to an older review, which the author has since
// seen superseded.
func ReadPrevious(ctx context.Context, client github.Client, owner, repo string, number int, viewer, source string) Previous {
	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number)
	none := func(reason string) Previous { return Previous{Schema: PreviousSchema, Reason: reason} }
	reviews, err := client.ListReviews(ctx, owner, repo, number)
	if err != nil {
		return none(fmt.Sprintf("could not list the reviews on %s: %v", prURL, err))
	}
	review, ok := newestOwn(reviews, viewer, source)
	if !ok {
		return none(fmt.Sprintf("no earlier loupe review from %s is on %s", publisher(viewer, source), prURL))
	}
	records, err := render.ReadRecord(review.Body)
	if err != nil {
		return none(fmt.Sprintf("%s cannot be read back: %v", review.HTMLURL, err))
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
		Round: render.MetaRound(review.Body), Findings: findings}
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
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", run.PreviousFile, err)
	}
	return append(data, '\n'), nil
}

// LoadPrevious reports found false only when previous.json does not exist, as for a run captured before it did or after
// a local receipt. A damaged file is a record refusal.
func LoadPrevious(dir string) (Previous, bool, error) {
	var p Previous
	found, err := loadRecord(filepath.Join(dir, run.PreviousFile), &p, func() string {
		switch {
		case p.Schema != PreviousSchema:
			return fmt.Sprintf("schema is %d, expected %d", p.Schema, PreviousSchema)
		case !p.Found && p.Reason == "":
			return "it holds neither findings nor a reason"
		}
		return ""
	})
	if found && err == nil && p.Found && p.Findings == nil {
		p.Findings = []EnvelopeFinding{}
	}
	return p, found, err
}
