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
