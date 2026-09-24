package publish

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/findingid"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
)

var (
	Actions     = []string{"comment", "approve", "request-changes"}
	InlineModes = []string{"none", "blocking", "all"}
)

var events = map[string]string{"comment": "COMMENT", "approve": "APPROVE", "request-changes": "REQUEST_CHANGES"}

// maxBodyChars is the owner's reading of GitHub's limit on a review or comment body; longer bodies are believed to be
// rejected with 422.
const (
	maxBodyChars = 65536
	limitFix     = "exclude a finding in loupe review or shorten bodies with loupe edit <id> --from -"
	// messageFix names the only place the message can be changed, because nothing else may write it.
	messageFix = "reword the message at the publish confirmation"
)

// BuildInput is everything a review is composed from. It is a struct rather than a parameter list because
// PublicationID and Message are adjacent strings the compiler cannot tell apart, and transposing them would publish
// a UUID as the review's opening prose.
type BuildInput struct {
	Target run.Target
	// Round is the published round the body shows; the envelope keeps the capture round, which receipts and run
	// references are keyed by.
	Round  int
	Draft  *draft.Draft
	Viewer string
	Action string
	Inline string
	// Unattended composes the whole publishable set, because nothing accepts a finding in a pipeline.
	Unattended bool
	// PublicationID is minted by the caller, so the reconciliation marker a human approves is the one that is sent.
	PublicationID string
	// Message is the human's own opening prose, typed at the confirmation. It fills the slot the draft's summary
	// fills unattended, and is empty when they typed none.
	Message string
	// Sticky composes a body later rounds edit in place; nil composes an ordinary review.
	Sticky *StickyBuild
}

// StickyBuild is the sticky review a round edits, or a zero Review when the round creates it.
type StickyBuild struct {
	Review github.Review
	// Rounds counts this round too.
	Rounds  int
	Earlier []string
}

// Build composes the review from the accepted findings, or, when unattended, the whole publishable set. It rechecks
// the allowlist because the draft file may have been edited by hand since the content was filed.
func Build(in BuildInput) (Envelope, error) {
	target, d := in.Target, in.Draft
	event, ok := events[in.Action]
	if !ok {
		return Envelope{}, fmt.Errorf("unknown review action %q", in.Action)
	}
	if !slices.Contains(InlineModes, in.Inline) {
		return Envelope{}, fmt.Errorf("unknown inline mode %q", in.Inline)
	}
	var included []draft.Finding
	if in.Unattended {
		included = draft.PublishableSet(d)
		slices.SortFunc(included, func(a, b draft.Finding) int { return findingid.Compare(a.ID, b.ID) })
	} else {
		included = accepted(d)
		if !sameIDs(included, draft.PublishableSet(d)) {
			return Envelope{}, refusal.New(refusal.NotReady, "the findings to publish are not all accepted", "loupe review")
		}
	}
	// Only the prose that is about to be published is checked. An attended publication does not post the draft's
	// summary, so refusing over it would be refusing over text no reader will see.
	opening, openingFix := in.Message, messageFix
	if in.Unattended {
		opening, openingFix = d.Summary, "loupe summary --from -"
	}
	if err := markdown.Check(opening, markdown.Summary, openingFix); err != nil {
		return Envelope{}, err
	}
	for _, f := range included {
		if err := markdown.Check(f.Body, markdown.Body, fmt.Sprintf("loupe edit %s --from -", f.ID)); err != nil {
			return Envelope{}, err
		}
		if f.Impact != "" {
			if err := markdown.Check(f.Impact, markdown.Body, fmt.Sprintf("loupe edit %s --from -", f.ID)); err != nil {
				return Envelope{}, err
			}
		}
		// References render as links whose <url> destination input validation keeps inert; a draft written by another
		// version is checked again here for the same reason the body is.
		if err := draft.ValidateReferences(f.References, fmt.Sprintf("loupe edit %s --from -", f.ID)); err != nil {
			return Envelope{}, err
		}
	}

	env := Envelope{
		Target:        EnvelopeTarget{Owner: target.Owner, Repo: target.Repo, Number: target.Number, HeadSHA: target.HeadSHA, Round: target.Round},
		Viewer:        in.Viewer,
		Action:        in.Action,
		Event:         event,
		CommitID:      target.HeadSHA,
		DraftVersion:  d.Version,
		Digest:        draft.Digest(d),
		PublicationID: in.PublicationID,
		Inline:        in.Inline,
		EditReviewID:  editReviewID(in.Sticky),
		Comments:      []Comment{},
		Findings:      []EnvelopeFinding{},
	}
	r := render.Input{Owner: target.Owner, Repo: target.Repo, Number: target.Number, Round: in.Round, HeadSHA: target.HeadSHA,
		Inline: in.Inline, Summary: opening, Digest: env.Digest, PublicationID: env.PublicationID, Source: target.Source, Model: target.Model, Unattended: in.Unattended}
	gate := draft.GateCountsOf(d)
	r.Excluded, r.Withdrawn, r.Reinstated, r.Regraded = gate.Excluded, gate.Withdrawn, gate.Reinstated, gate.Regraded
	for _, f := range included {
		rf := render.Finding{ID: f.ID, Title: f.Title, Body: f.Body, General: f.General, Label: f.Label, Blocking: f.Blocking,
			Confidence: f.Confidence, Severity: f.Severity, Verified: f.Verified, Impact: f.Impact, References: f.References, SuggestedFix: f.SuggestedFix}
		var loc *draft.Location
		if f.Location != nil {
			rf.Location = &render.Location{Path: f.Location.Path, Side: f.Location.Side, Line: f.Location.Line, StartLine: f.Location.StartLine}
			copied := *f.Location
			loc = &copied
		}
		r.Findings = append(r.Findings, rf)
		env.Findings = append(env.Findings, EnvelopeFinding{ID: f.ID, Title: f.Title, Body: f.Body, Location: loc, Label: f.Label, Blocking: f.Blocking})
	}

	// One finding per call ties each comment to its id; included is already in the order Comments sorts by.
	for _, rf := range r.Findings {
		single := r
		single.Findings = []render.Finding{rf}
		for _, c := range render.Comments(single) {
			if n := utf8.RuneCountInString(c.Body); n > maxBodyChars {
				return Envelope{}, limitRefusal(fmt.Sprintf("the inline comment for %s is %d characters; at most %d characters are allowed", rf.ID, n, maxBodyChars))
			}
			env.Comments = append(env.Comments, Comment{Path: c.Path, Line: c.Line, Side: c.Side, StartLine: c.StartLine, StartSide: c.StartSide, Body: c.Body})
		}
	}
	if in.Sticky != nil {
		r.Sticky = &render.StickyInput{Rounds: in.Sticky.Rounds, Earlier: in.Sticky.Earlier}
	}
	env.Body = render.Body(r)
	// The oldest collapsed rounds give way first, so a pull request with many rounds never stops a sticky review.
	for r.Sticky != nil && len(r.Sticky.Earlier) > 0 && utf8.RuneCountInString(env.Body) > maxBodyChars {
		r.Sticky.Earlier = r.Sticky.Earlier[:len(r.Sticky.Earlier)-1]
		env.Body = render.Body(r)
	}
	if n := utf8.RuneCountInString(env.Body); n > maxBodyChars {
		return Envelope{}, limitRefusal(fmt.Sprintf("the composed review body is %d characters; at most %d characters are allowed", n, maxBodyChars))
	}
	return env, nil
}

func editReviewID(s *StickyBuild) int64 {
	if s == nil {
		return 0
	}
	return s.Review.ID
}

func limitRefusal(message string) error {
	r := refusal.New(refusal.Markdown, message, limitFix)
	r.Details = map[string]any{"rule": "limit"}
	return r
}

// accepted is sorted by id so the envelope's findings are in a stable order.
func accepted(d *draft.Draft) []draft.Finding {
	dispositions := draft.Dispositions(d)
	var out []draft.Finding
	for _, f := range d.Findings {
		if dispositions[f.ID] == draft.DispositionAccepted {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(a, b draft.Finding) int { return findingid.Compare(a.ID, b.ID) })
	return out
}

func sameIDs(a, b []draft.Finding) bool {
	ids := func(fs []draft.Finding) []string {
		out := make([]string, 0, len(fs))
		for _, f := range fs {
			out = append(out, f.ID)
		}
		slices.SortFunc(out, findingid.Compare)
		return out
	}
	return slices.Equal(ids(a), ids(b))
}
