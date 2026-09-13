package draft

import (
	"fmt"
	"strings"
	"time"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

// FindingInput is the add input in contracts/cli.md. It has no included field because only the human changes that.
type FindingInput struct {
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	Location     *Location `json:"location,omitempty"`
	General      bool      `json:"general,omitempty"`
	Label        string    `json:"label,omitempty"`
	Blocking     bool      `json:"blocking,omitempty"`
	Confidence   string    `json:"confidence,omitempty"`
	Severity     string    `json:"severity,omitempty"`
	SuggestedFix string    `json:"suggestedFix,omitempty"`
}

const inputFix = "see loupe add --help for the input shape"

// Add validates every entry before appending any, so a batch is stored entirely or not at all.
func Add(d *Draft, inputs []FindingInput, dif *diff.Diff, by string, now time.Time) ([]Finding, error) {
	if by == "" {
		by = ByAgent
	}
	for i, in := range inputs {
		if err := validateInput(in, dif); err != nil {
			return nil, atEntry(err, i)
		}
	}
	added := make([]Finding, 0, len(inputs))
	for _, in := range inputs {
		f := Finding{
			ID:           NextFindingID(d),
			Rev:          1,
			Title:        in.Title,
			Body:         in.Body,
			General:      in.General,
			Label:        in.Label,
			Blocking:     in.Blocking,
			Confidence:   in.Confidence,
			Severity:     in.Severity,
			SuggestedFix: in.SuggestedFix,
			By:           by,
			Included:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
			History:      []HistoryEntry{},
		}
		if in.Location != nil {
			loc := *in.Location
			if loc.Side == "" {
				loc.Side = SideRight
			}
			f.Location = &loc
		}
		d.Findings = append(d.Findings, f)
		added = append(added, f)
	}
	return added, nil
}

func validateInput(in FindingInput, dif *diff.Diff) error {
	switch {
	case strings.TrimSpace(in.Title) == "":
		return refusal.New(refusal.Input, "title is required and must not be empty", inputFix)
	case strings.TrimSpace(in.Body) == "":
		return refusal.New(refusal.Input, "body is required and must not be empty", inputFix)
	case in.Location != nil && in.General:
		return refusal.New(refusal.Input, `a finding has either location or "general": true, not both`, inputFix)
	case in.Location == nil && !in.General:
		return refusal.New(refusal.Input, `a finding needs a location or "general": true`, inputFix)
	}
	switch in.Confidence {
	case "", "high", "medium", "low":
	default:
		return refusal.New(refusal.Input, fmt.Sprintf("confidence %q must be high, medium or low", in.Confidence), inputFix)
	}
	if err := markdown.Check(in.Body, markdown.Body, "loupe edit <id> --from -"); err != nil {
		return err
	}
	if in.Location == nil {
		return nil
	}
	side := in.Location.Side
	switch side {
	case "":
		side = SideRight
	case SideRight, SideLeft:
	default:
		return refusal.New(refusal.Input, fmt.Sprintf("side %q must be RIGHT or LEFT", side), inputFix)
	}
	return dif.Validate(in.Location.Path, side, in.Location.Line, in.Location.StartLine)
}

// atEntry names the zero-based batch entry a refusal came from.
func atEntry(err error, entry int) error {
	r, ok := refusal.As(err)
	if !ok {
		return err
	}
	details := map[string]any{"entry": entry}
	for k, v := range r.Details {
		details[k] = v
	}
	return &refusal.Error{Code: r.Code, Message: fmt.Sprintf("entry %d: %s", entry, r.Message), Fix: r.Fix, Details: details}
}

// SetSummary compares expectFindings with the included count so an agent cannot summarize a set of findings that
// did not all land.
func SetSummary(d *Draft, summary string, expectFindings *int, by string) error {
	if err := markdown.Check(summary, markdown.Summary, "loupe summary --from -"); err != nil {
		return err
	}
	if expectFindings != nil {
		if n := IncludedCount(d); n != *expectFindings {
			included := make([]map[string]string, 0, n)
			ids := make([]string, 0, n)
			for _, f := range d.Findings {
				if f.Included {
					included = append(included, map[string]string{"id": f.ID, "title": f.Title})
					ids = append(ids, f.ID)
				}
			}
			fix := "no findings are included; add them with loupe add, then retry"
			if n > 0 {
				fix = fmt.Sprintf("included findings are %s; add the missing findings with loupe add or retry with --expect-findings %d", strings.Join(ids, ", "), n)
			}
			r := refusal.New(refusal.Count, fmt.Sprintf("%d findings are included, expected %d", n, *expectFindings), fix)
			r.Details = map[string]any{"included": included}
			return r
		}
	}
	d.Summary = summary
	return nil
}

const noteFix = "reword the note and send the finding back again"

func findFinding(d *Draft, id string) (*Finding, error) {
	for i := range d.Findings {
		if d.Findings[i].ID == id {
			return &d.Findings[i], nil
		}
	}
	return nil, refusal.New(refusal.NotFound, fmt.Sprintf("finding %s does not exist", id), "loupe show")
}

func findNote(d *Draft, id string) (*Note, error) {
	for i := range d.Notes {
		if d.Notes[i].ID == id {
			return &d.Notes[i], nil
		}
	}
	return nil, refusal.New(refusal.NotFound, fmt.Sprintf("note %s does not exist", id), "loupe show")
}

// Accept records the decision at the finding's current rev, so any later edit puts the finding back to pending.
func Accept(d *Draft, findingID string, now time.Time) error {
	f, err := findFinding(d, findingID)
	if err != nil {
		return err
	}
	if !f.Included {
		return refusal.New(refusal.Input, "accept is for included findings only",
			fmt.Sprintf("exclude %s instead, or have the agent restore it with loupe edit %s --include", f.ID, f.ID))
	}
	d.Decisions[f.ID] = Decision{FindingID: f.ID, Decision: DecisionAccepted, FindingRev: f.Rev, At: now}
	return nil
}

func Exclude(d *Draft, findingID string, now time.Time) error {
	f, err := findFinding(d, findingID)
	if err != nil {
		return err
	}
	d.Decisions[f.ID] = Decision{FindingID: f.ID, Decision: DecisionExcluded, FindingRev: f.Rev, At: now}
	return nil
}

// SendBack deletes the finding's decision so it stays pending until the human decides it again.
func SendBack(d *Draft, findingID, body string, now time.Time) (Note, error) {
	f, err := findFinding(d, findingID)
	if err != nil {
		return Note{}, err
	}
	if strings.TrimSpace(body) == "" {
		return Note{}, refusal.New(refusal.Input, "a send-back note must not be empty", noteFix)
	}
	if err := markdown.Check(body, markdown.Body, noteFix); err != nil {
		return Note{}, err
	}
	n := Note{ID: NextNoteID(d), FindingID: f.ID, Body: body, At: now, Status: NoteOpen}
	d.Notes = append(d.Notes, n)
	delete(d.Decisions, f.ID)
	return n, nil
}

func Restore(d *Draft, findingID string) error {
	f, err := findFinding(d, findingID)
	if err != nil {
		return err
	}
	if disposition(d, *f) != DispositionExcluded {
		return refusal.New(refusal.Input, fmt.Sprintf("restore is for excluded findings only; %s is %s", f.ID, disposition(d, *f)),
			"accept, exclude or send back the finding instead")
	}
	delete(d.Decisions, f.ID)
	return nil
}

func ResolveNote(d *Draft, noteID string, now time.Time) error {
	return closeNote(d, noteID, NoteResolved, now)
}

func DismissNote(d *Draft, noteID string, now time.Time) error {
	return closeNote(d, noteID, NoteDismissed, now)
}

func closeNote(d *Draft, noteID, status string, now time.Time) error {
	n, err := findNote(d, noteID)
	if err != nil {
		return err
	}
	if n.Status != NoteOpen {
		return refusal.New(refusal.Input, fmt.Sprintf("note %s is already %s", n.ID, n.Status), "only an open note can be resolved or dismissed")
	}
	n.Status = status
	n.ClosedAt = &now
	return nil
}
