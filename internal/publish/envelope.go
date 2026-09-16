package publish

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/findingid"
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
)

// Build composes the review from the accepted findings, or, when unattended, the whole publishable set: nothing
// accepts a finding in a pipeline. It rechecks the allowlist because the draft file may have been edited by hand
// since the content was filed. round is the published round the body shows; the envelope keeps the capture round,
// which receipts and run references are keyed by.
func Build(target run.Target, round int, d *draft.Draft, viewer, action, inline string, unattended bool) (Envelope, error) {
	event, ok := events[action]
	if !ok {
		return Envelope{}, fmt.Errorf("unknown review action %q", action)
	}
	if !slices.Contains(InlineModes, inline) {
		return Envelope{}, fmt.Errorf("unknown inline mode %q", inline)
	}
	var included []draft.Finding
	if unattended {
		included = draft.PublishableSet(d)
		slices.SortFunc(included, func(a, b draft.Finding) int { return findingid.Compare(a.ID, b.ID) })
	} else {
		included = accepted(d)
		if !sameIDs(included, draft.PublishableSet(d)) {
			return Envelope{}, refusal.New(refusal.NotReady, "the findings to publish are not all accepted", "loupe review")
		}
	}
	if err := markdown.Check(d.Summary, markdown.Summary, "loupe summary --from -"); err != nil {
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
		// References render as autolinks that input validation keeps inert; a draft written by another version is
		// checked again here for the same reason the body is.
		if err := draft.ValidateReferences(f.References, fmt.Sprintf("loupe edit %s --from -", f.ID)); err != nil {
			return Envelope{}, err
		}
	}

	env := Envelope{
		Target:        EnvelopeTarget{Owner: target.Owner, Repo: target.Repo, Number: target.Number, HeadSHA: target.HeadSHA, Round: target.Round},
		Viewer:        viewer,
		Action:        action,
		Event:         event,
		CommitID:      target.HeadSHA,
		DraftVersion:  d.Version,
		Digest:        draft.Digest(d),
		PublicationID: newPublicationID(),
		Inline:        inline,
		Comments:      []Comment{},
		Findings:      []EnvelopeFinding{},
	}
	in := render.Input{Owner: target.Owner, Repo: target.Repo, Number: target.Number, Round: round, HeadSHA: target.HeadSHA,
		Inline: inline, Summary: d.Summary, Digest: env.Digest, PublicationID: env.PublicationID, Source: target.Source, Model: target.Model, Unattended: unattended}
	for _, f := range included {
		rf := render.Finding{ID: f.ID, Title: f.Title, Body: f.Body, General: f.General, Label: f.Label, Blocking: f.Blocking,
			Confidence: f.Confidence, Severity: f.Severity, Verified: f.Verified, Impact: f.Impact, References: f.References, SuggestedFix: f.SuggestedFix}
		var loc *draft.Location
		if f.Location != nil {
			rf.Location = &render.Location{Path: f.Location.Path, Side: f.Location.Side, Line: f.Location.Line, StartLine: f.Location.StartLine}
			copied := *f.Location
			loc = &copied
		}
		in.Findings = append(in.Findings, rf)
		env.Findings = append(env.Findings, EnvelopeFinding{ID: f.ID, Title: f.Title, Body: f.Body, Location: loc, Label: f.Label, Blocking: f.Blocking})
	}

	// One finding per call ties each comment to its id; included is already in the order Comments sorts by.
	for _, rf := range in.Findings {
		single := in
		single.Findings = []render.Finding{rf}
		for _, c := range render.Comments(single) {
			if n := utf8.RuneCountInString(c.Body); n > maxBodyChars {
				return Envelope{}, limitRefusal(fmt.Sprintf("the inline comment for %s is %d characters; at most %d characters are allowed", rf.ID, n, maxBodyChars))
			}
			env.Comments = append(env.Comments, Comment{Path: c.Path, Line: c.Line, Side: c.Side, StartLine: c.StartLine, StartSide: c.StartSide, Body: c.Body})
		}
	}
	env.Body = render.Body(in)
	if n := utf8.RuneCountInString(env.Body); n > maxBodyChars {
		return Envelope{}, limitRefusal(fmt.Sprintf("the composed review body is %d characters; at most %d characters are allowed", n, maxBodyChars))
	}
	return env, nil
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
