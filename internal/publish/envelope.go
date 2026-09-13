package publish

import (
	"fmt"
	"slices"

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

const maxBodyBytes = 256 * 1024

// Build composes the review from the accepted findings only. It rechecks the allowlist because the draft file may
// have been edited by hand since the content was filed.
func Build(target run.Target, d *draft.Draft, viewer, action, inline string) (Envelope, error) {
	event, ok := events[action]
	if !ok {
		return Envelope{}, fmt.Errorf("unknown review action %q", action)
	}
	if !slices.Contains(InlineModes, inline) {
		return Envelope{}, fmt.Errorf("unknown inline mode %q", inline)
	}
	included := accepted(d)
	if !sameIDs(included, draft.PublishableSet(d)) {
		return Envelope{}, refusal.New(refusal.NotReady, "the findings to publish are not all accepted", "loupe review")
	}
	if err := markdown.Check(d.Summary, markdown.Summary, "loupe summary --from -"); err != nil {
		return Envelope{}, err
	}
	for _, f := range included {
		if err := markdown.Check(f.Body, markdown.Body, fmt.Sprintf("loupe edit %s --from -", f.ID)); err != nil {
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
	in := render.Input{Owner: target.Owner, Repo: target.Repo, Number: target.Number, Round: target.Round, HeadSHA: target.HeadSHA,
		Action: action, Inline: inline, Summary: d.Summary, Digest: env.Digest, PublicationID: env.PublicationID}
	for _, f := range included {
		rf := render.Finding{ID: f.ID, Title: f.Title, Body: f.Body, General: f.General, Label: f.Label, Blocking: f.Blocking,
			Confidence: f.Confidence, Severity: f.Severity, SuggestedFix: f.SuggestedFix}
		var loc *draft.Location
		if f.Location != nil {
			rf.Location = &render.Location{Path: f.Location.Path, Side: f.Location.Side, Line: f.Location.Line, StartLine: f.Location.StartLine}
			copied := *f.Location
			loc = &copied
		}
		in.Findings = append(in.Findings, rf)
		env.Findings = append(env.Findings, EnvelopeFinding{ID: f.ID, Title: f.Title, Body: f.Body, Location: loc, Label: f.Label, Blocking: f.Blocking})
	}

	env.Body = render.Body(in)
	if len(env.Body) > maxBodyBytes {
		r := refusal.New(refusal.Markdown,
			fmt.Sprintf("the composed review body is %d bytes; at most %d bytes (256 KiB) are allowed", len(env.Body), maxBodyBytes),
			"exclude a finding with loupe edit <id> --exclude or shorten bodies")
		r.Details = map[string]any{"rule": "limit"}
		return Envelope{}, r
	}
	for _, c := range render.Comments(in) {
		env.Comments = append(env.Comments, Comment{Path: c.Path, Line: c.Line, Side: c.Side, StartLine: c.StartLine, StartSide: c.StartSide, Body: c.Body})
	}
	return env, nil
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
