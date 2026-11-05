package mode

import (
	"path/filepath"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

// Wait reasons: what ended a run for the agent.
const (
	EndedPublished = "published"
	EndedReturned  = "returned"
)

// prFeedback is what loupe feedback and loupe wait return on every run: notes with their replies, each finding's
// disposition, and readiness.
func prFeedback(d *draft.Draft) map[string]any {
	notes := make([]map[string]any, 0, len(d.Notes))
	for _, n := range d.Notes {
		notes = append(notes, map[string]any{"id": n.ID, "findingId": n.FindingID, "status": n.Status, "body": n.Body, "at": n.At,
			"replies": RepliesTo(d, n.ID)})
	}
	dispositions := draft.Dispositions(d)
	findings := make([]map[string]any, 0, len(d.Findings))
	for _, f := range d.Findings {
		entry := map[string]any{"id": f.ID, "title": f.Title, "disposition": dispositions[f.ID], "rev": f.Rev}
		if reason := draft.CurrentReason(f); reason != nil {
			entry["withdrawReason"] = reason
		}
		findings = append(findings, entry)
	}
	return map[string]any{
		"readiness": draft.ReadinessOf(d),
		"notes":     notes,
		"findings":  findings,
	}
}

// RepliesTo lists the replies to the note noteID, oldest first.
func RepliesTo(d *draft.Draft, noteID string) []draft.Reply {
	out := []draft.Reply{}
	for _, r := range d.Replies {
		if r.NoteID == noteID {
			out = append(out, r)
		}
	}
	return out
}

func (publish) Feedback(d *draft.Draft, _ string) (map[string]any, error) { return prFeedback(d), nil }

func (publish) Ended(dir string) (string, error) {
	published, err := run.HasReceipt(dir)
	if err != nil || !published {
		return "", err
	}
	return EndedPublished, nil
}

func (apply) Ended(dir string) (string, error) {
	returned, err := run.HasReturn(dir)
	if err != nil || !returned {
		return "", err
	}
	return EndedReturned, nil
}

// Feedback adds to the pull request payload whether the run was returned, and its decided findings in the groups the
// agent acts on: apply as proposed, apply as the human changed it, leave out, or nothing to do.
func (a apply) Feedback(d *draft.Draft, dir string) (map[string]any, error) {
	payload := prFeedback(d)
	// The draft was read before return.json, so a seal of a newer draft does not make d the returned decisions.
	sealed, returned, err := draft.LoadReturn(dir)
	if err != nil {
		return nil, err
	}
	payload["returned"] = returned && sealed.DraftVersion == d.Version
	groups := map[string][]map[string]any{"apply": {}, "applyWithChanges": {}, "excluded": {}, "withdrawn": {}}
	for _, f := range d.Findings {
		entry := a.entry(d, f)
		decision := d.Decisions[f.ID]
		switch draft.GateOf(d, f).Disposition {
		case draft.DispositionAccepted:
			if decision.Note != "" || draft.HumanEdited(f) {
				if decision.Note != "" {
					entry["suggestion"] = decision.Note
				}
				groups["applyWithChanges"] = append(groups["applyWithChanges"], entry)
				continue
			}
			groups["apply"] = append(groups["apply"], entry)
		case draft.DispositionExcluded:
			if decision.Decision == draft.DecisionExcluded && decision.FindingRev == f.Rev && decision.Note != "" {
				entry["reason"] = decision.Note
			} else if reason := draft.CurrentReason(f); reason != nil && reason.Text != "" {
				entry["reason"] = reason.Text
			}
			groups["excluded"] = append(groups["excluded"], entry)
		case draft.DispositionWithdrawn:
			if reason := draft.CurrentReason(f); reason != nil {
				entry["withdrawReason"] = reason
			}
			groups["withdrawn"] = append(groups["withdrawn"], entry)
		}
	}
	payload["groups"] = groups
	return payload, nil
}

// entry is one finding as the agent applies it: no blocking or side, the absolute path to edit, and its notes.
func (a apply) entry(d *draft.Draft, f draft.Finding) map[string]any {
	e := map[string]any{"id": f.ID, "title": f.Title, "body": f.Body, "general": f.General, "thread": thread(d, f.ID)}
	for k, v := range map[string]string{"proposal": f.Proposal, "label": f.Label, "severity": f.Severity} {
		if v != "" {
			e[k] = v
		}
	}
	if f.Location != nil {
		loc := map[string]any{"path": f.Location.Path, "line": f.Location.Line}
		if f.Location.StartLine != 0 {
			loc["startLine"] = f.Location.StartLine
		}
		if f.Location.Heading != "" {
			loc["heading"] = f.Location.Heading
		}
		e["location"] = loc
		e["path"] = a.absPath(f.Location.Path)
	}
	return e
}

func (a apply) absPath(display string) string {
	l := a.target.Local
	if l == nil {
		return display
	}
	for _, file := range l.Files {
		if file.Path == display {
			return file.Abs
		}
	}
	return filepath.Join(l.Root, filepath.FromSlash(display))
}

func thread(d *draft.Draft, findingID string) []map[string]any {
	out := []map[string]any{}
	for _, n := range d.Notes {
		if n.FindingID != findingID {
			continue
		}
		replies := []map[string]any{}
		for _, r := range RepliesTo(d, n.ID) {
			replies = append(replies, map[string]any{"id": r.ID, "body": r.Body, "by": r.By, "at": r.At})
		}
		out = append(out, map[string]any{"id": n.ID, "body": n.Body, "at": n.At, "status": n.Status, "replies": replies})
	}
	return out
}
