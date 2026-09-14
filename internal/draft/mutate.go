package draft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

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

// LabelPattern keeps labels free of Markdown punctuation, because an inline comment's summary line is a Markdown
// paragraph where the label is not escaped.
const LabelPattern = `^[\p{L}\p{N}][\p{L}\p{N}_.-]*$`

const maxLabelRunes = 40

var labelRE = regexp.MustCompile(LabelPattern)

// ValidateLabel accepts the empty label, which means no label.
func ValidateLabel(label string) error {
	if label == "" || (labelRE.MatchString(label) && utf8.RuneCountInString(label) <= maxLabelRunes) {
		return nil
	}
	return refusal.New(refusal.Input,
		fmt.Sprintf("label %q must match %s and be at most %d characters", label, LabelPattern, maxLabelRunes), inputFix)
}

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
	if err := ValidateLabel(in.Label); err != nil {
		return err
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

// EditInput holds each field as raw JSON so an absent key (nil), a null and a value stay distinguishable.
type EditInput struct {
	Title        json.RawMessage `json:"title"`
	Body         json.RawMessage `json:"body"`
	Location     json.RawMessage `json:"location"`
	General      json.RawMessage `json:"general"`
	Label        json.RawMessage `json:"label"`
	Blocking     json.RawMessage `json:"blocking"`
	Confidence   json.RawMessage `json:"confidence"`
	Severity     json.RawMessage `json:"severity"`
	SuggestedFix json.RawMessage `json:"suggestedFix"`
}

const editFix = "see loupe edit --help for the input shape"

// Edit applies in and, when included is not nil, sets Included. A change to a publishable field or to Included bumps
// rev, deletes the decision and appends one history entry; cleared reports whether a decision was deleted. dif is
// needed only when in changes a publishable field. An edit that changes nothing returns the finding with ErrNoChange.
func Edit(d *Draft, findingID string, in EditInput, included *bool, dif *diff.Diff, by string, now time.Time) (f Finding, cleared bool, err error) {
	stored, err := findFinding(d, findingID)
	if err != nil {
		return Finding{}, false, err
	}
	if by == "" {
		by = ByAgent
	}
	next := *stored
	if err := applyEdit(&next, in); err != nil {
		return Finding{}, false, err
	}
	if included != nil {
		next.Included = *included
	}

	changed := map[string]any{}
	note := func(field string, differs bool, previous any) {
		if differs {
			changed[field] = previous
		}
	}
	note("title", next.Title != stored.Title, stored.Title)
	note("body", next.Body != stored.Body, stored.Body)
	note("location", !sameLocation(next.Location, stored.Location), stored.Location)
	note("general", next.General != stored.General, stored.General)
	note("label", next.Label != stored.Label, stored.Label)
	note("blocking", next.Blocking != stored.Blocking, stored.Blocking)
	note("confidence", next.Confidence != stored.Confidence, stored.Confidence)
	note("severity", next.Severity != stored.Severity, stored.Severity)
	note("suggestedFix", next.SuggestedFix != stored.SuggestedFix, stored.SuggestedFix)
	publishable := len(changed) > 0
	note("included", next.Included != stored.Included, stored.Included)
	if len(changed) == 0 {
		return *stored, false, ErrNoChange
	}
	if publishable {
		err := validateInput(FindingInput{Title: next.Title, Body: next.Body, Location: next.Location, General: next.General, Label: next.Label,
			Blocking: next.Blocking, Confidence: next.Confidence, Severity: next.Severity, SuggestedFix: next.SuggestedFix}, dif)
		if err != nil {
			return Finding{}, false, err
		}
	}

	next.Rev++
	next.UpdatedAt = now
	next.History = append(append([]HistoryEntry{}, stored.History...), HistoryEntry{At: now, By: by, Changed: changed})
	_, cleared = d.Decisions[next.ID]
	delete(d.Decisions, next.ID)
	*stored = next
	return next, cleared, nil
}

// Withdraw takes a finding out of the review without recording a human decision.
func Withdraw(d *Draft, findingID, by string, now time.Time) (Finding, bool, error) {
	included := false
	return Edit(d, findingID, EditInput{}, &included, nil, by, now)
}

// Include restores a withdrawn finding; it is pending until the human decides it again.
func Include(d *Draft, findingID, by string, now time.Time) (Finding, bool, error) {
	included := true
	return Edit(d, findingID, EditInput{}, &included, nil, by, now)
}

func applyEdit(f *Finding, in EditInput) error {
	if err := decodeRequired("title", in.Title, &f.Title); err != nil {
		return err
	}
	if err := decodeRequired("body", in.Body, &f.Body); err != nil {
		return err
	}
	if err := decodeRequired("blocking", in.Blocking, &f.Blocking); err != nil {
		return err
	}
	for _, field := range []struct {
		name string
		raw  json.RawMessage
		dst  *string
	}{{"label", in.Label, &f.Label}, {"confidence", in.Confidence, &f.Confidence}, {"severity", in.Severity, &f.Severity},
		{"suggestedFix", in.SuggestedFix, &f.SuggestedFix}} {
		if err := decodeOptional(field.name, field.raw, field.dst); err != nil {
			return err
		}
	}

	switch {
	case in.Location == nil:
	case isNull(in.Location):
		// A finding needs a location or general, so clearing the location makes it general.
		f.Location, f.General = nil, true
	default:
		var loc Location
		if err := decodeValue("location", in.Location, &loc); err != nil {
			return err
		}
		if loc.Side == "" {
			loc.Side = SideRight
		}
		f.Location, f.General = &loc, false
	}
	if in.General != nil {
		var general bool
		if err := decodeRequired("general", in.General, &general); err != nil {
			return err
		}
		f.General = general
		if general && in.Location == nil {
			f.Location = nil
		}
	}
	return nil
}

func isNull(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }

func decodeRequired(field string, raw json.RawMessage, dst any) error {
	if raw == nil {
		return nil
	}
	if isNull(raw) {
		return refusal.New(refusal.Input, fmt.Sprintf("%s cannot be null; only location, label, confidence, severity and suggestedFix can be cleared", field), editFix)
	}
	return decodeValue(field, raw, dst)
}

func decodeOptional(field string, raw json.RawMessage, dst *string) error {
	switch {
	case raw == nil:
		return nil
	case isNull(raw):
		*dst = ""
		return nil
	}
	return decodeValue(field, raw, dst)
}

func decodeValue(field string, raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return refusal.New(refusal.Input, fmt.Sprintf("input field %q is not valid: %v", field, err), editFix)
	}
	return nil
}

func sameLocation(a, b *Location) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// AddReply answers a note. It never changes the note's status or any decision; only the human closes notes.
func AddReply(d *Draft, noteID, body, by string, now time.Time) (Reply, error) {
	n, err := findNote(d, noteID)
	if err != nil {
		return Reply{}, err
	}
	fix := fmt.Sprintf("loupe reply %s --from -", n.ID)
	if strings.TrimSpace(body) == "" {
		return Reply{}, refusal.New(refusal.Input, "a reply must not be empty", fix)
	}
	if err := markdown.Check(body, markdown.Body, fix); err != nil {
		return Reply{}, err
	}
	if by == "" {
		by = ByAgent
	}
	r := Reply{ID: NextReplyID(d), NoteID: n.ID, Body: body, By: by, At: now}
	d.Replies = append(d.Replies, r)
	return r, nil
}
