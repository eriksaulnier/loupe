package draft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/severity"
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
	Verified     string    `json:"verified,omitempty"`
	Impact       string    `json:"impact,omitempty"`
	References   []string  `json:"references,omitempty"`
	SuggestedFix string    `json:"suggestedFix,omitempty"`
}

const inputFix = "see loupe add --help for the input shape"

// LabelPattern keeps labels free of Markdown punctuation, because an inline comment's summary line is a Markdown
// paragraph. The renderer escapes the label there too, so this is a second line of defense, not the only one.
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

const maxReferences = 6

const maxReferenceBytes = 200

// ValidateReferences accepts an empty list. A reference is rendered as a Markdown link to <url>, so whitespace,
// control characters, angle brackets and backticks are refused: any of them would end or break the destination. fix
// names the command that repairs the finding, which differs between add and a recheck at publish.
func ValidateReferences(refs []string, fix string) error {
	if len(refs) > maxReferences {
		return refusal.New(refusal.Input, fmt.Sprintf("references has %d entries; at most %d are allowed", len(refs), maxReferences), fix)
	}
	for i, ref := range refs {
		if err := validateReference(ref); err != nil {
			return refusal.New(refusal.Input, fmt.Sprintf("references[%d] %q %s", i, ref, err), fix)
		}
	}
	return nil
}

func validateReference(ref string) error {
	if len(ref) > maxReferenceBytes {
		return fmt.Errorf("is over %d bytes", maxReferenceBytes)
	}
	for _, r := range ref {
		// Format characters cover the bidi overrides that make a URL display a host it does not point to.
		if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '<' || r == '>' || r == '`' {
			return errors.New("must not contain whitespace, control or format characters, <, > or `")
		}
	}
	u, err := url.Parse(ref)
	// Hostname, not Host: url.Parse gives https://:80 the Host ":80", which names no host.
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return errors.New("must be an http or https URL with a host")
	}
	// user@host reads as the user part's host to anyone who stops at the first slash.
	if u.User != nil {
		return errors.New("must not carry userinfo before the host")
	}
	return nil
}

func validateSeverity(word string) error {
	if word == "" || slices.Contains(severity.Order[:], word) {
		return nil
	}
	return refusal.New(refusal.Input, fmt.Sprintf("severity %q must be critical, major, minor or trivial", word), inputFix)
}

// Add validates every entry before appending any, so a batch is stored entirely or not at all.
func Add(d *Draft, inputs []FindingInput, dif *diff.Diff, by string, now time.Time) ([]Finding, error) {
	if by == "" {
		by = ByAgent
	}
	if err := CheckBatch(inputs, nil, dif); err != nil {
		return nil, err
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
			Verified:     in.Verified,
			Impact:       in.Impact,
			SuggestedFix: in.SuggestedFix,
			By:           by,
			Included:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
			History:      []HistoryEntry{},
		}
		if len(in.References) > 0 {
			f.References = slices.Clone(in.References)
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
	if err := validateSeverity(in.Severity); err != nil {
		return err
	}
	switch in.Verified {
	case "", "reproduced", "plausible":
	default:
		return refusal.New(refusal.Input, fmt.Sprintf("verified %q must be reproduced or plausible", in.Verified), inputFix)
	}
	if err := ValidateReferences(in.References, inputFix); err != nil {
		return err
	}
	if err := markdown.Check(in.Body, markdown.Body, "loupe edit <id> --from -"); err != nil {
		return err
	}
	if in.Impact != "" {
		if err := markdown.Check(in.Impact, markdown.Body, "loupe edit <id> --from -"); err != nil {
			return err
		}
	}
	if in.SuggestedFix != "" {
		if err := markdown.Check(in.SuggestedFix, markdown.Body, "loupe edit <id> --from -"); err != nil {
			return err
		}
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

// EntryFailure is one batch entry that was refused before or during validation.
type EntryFailure struct {
	Entry int
	Err   error
	// Decoding marks a failure to read the entry as a finding at all, which the message names as an input entry.
	Decoding bool
}

// CheckBatch validates every input not already in failed and refuses once, naming every invalid entry, so a caller
// can fix the whole batch in one retry. The top level is the first invalid entry's own refusal.
func CheckBatch(inputs []FindingInput, failed []EntryFailure, dif *diff.Diff) error {
	known := make(map[int]bool, len(failed))
	for _, f := range failed {
		known[f.Entry] = true
	}
	failures := slices.Clone(failed)
	for i, in := range inputs {
		if known[i] {
			continue
		}
		if err := validateInput(in, dif); err != nil {
			failures = append(failures, EntryFailure{Entry: i, Err: err})
		}
	}
	if len(failures) == 0 {
		return nil
	}
	slices.SortFunc(failures, func(a, b EntryFailure) int { return a.Entry - b.Entry })
	entries := make([]map[string]any, 0, len(failures))
	for _, f := range failures {
		r, ok := refusal.As(f.Err)
		if !ok {
			return f.Err
		}
		entry := map[string]any{"entry": f.Entry, "code": string(r.Code), "message": r.Message, "fix": r.Fix}
		if len(r.Details) > 0 {
			entry["details"] = r.Details
		}
		entries = append(entries, entry)
	}
	first, _ := refusal.As(failures[0].Err)
	details := map[string]any{}
	for k, v := range first.Details {
		details[k] = v
	}
	details["entry"], details["entries"] = failures[0].Entry, entries
	prefix := "entry"
	if failures[0].Decoding {
		prefix = "input entry"
	}
	message := fmt.Sprintf("%s %d: %s%s", prefix, failures[0].Entry, first.Message, alsoRefused(failures[1:]))
	return &refusal.Error{Code: first.Code, Message: message, Fix: first.Fix, Details: details}
}

// alsoRefused tells a reader of the message alone that the first refused entry is not the only one.
func alsoRefused(rest []EntryFailure) string {
	switch len(rest) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("; entry %d is refused too", rest[0].Entry)
	}
	nums := make([]string, len(rest))
	for i, f := range rest {
		nums[i] = strconv.Itoa(f.Entry)
	}
	return fmt.Sprintf("; entries %s and %s are refused too", strings.Join(nums[:len(nums)-1], ", "), nums[len(nums)-1])
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

// Accept records the decision at the finding's current rev, so any later edit puts the finding back to pending. It
// resolves the finding's open notes, since accepting is the human's answer to them, and returns their ids.
func Accept(d *Draft, findingID string, now time.Time) ([]string, error) {
	f, err := findFinding(d, findingID)
	if err != nil {
		return nil, err
	}
	if !f.Included {
		return nil, refusal.New(refusal.Input, "accept is for included findings only",
			fmt.Sprintf("reinstate %s to accept it, or exclude it instead", f.ID))
	}
	d.Decisions[f.ID] = Decision{FindingID: f.ID, Decision: DecisionAccepted, FindingRev: f.Rev, At: now}
	return closeOpenNotes(d, f.ID, NoteResolved, now), nil
}

// Exclude dismisses the finding's open notes and returns their ids.
func Exclude(d *Draft, findingID string, now time.Time) ([]string, error) {
	f, err := findFinding(d, findingID)
	if err != nil {
		return nil, err
	}
	d.Decisions[f.ID] = Decision{FindingID: f.ID, Decision: DecisionExcluded, FindingRev: f.Rev, At: now}
	return closeOpenNotes(d, f.ID, NoteDismissed, now), nil
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

// Reinstate is the human's override of an agent withdrawal, from the review interface: it puts the finding back in
// the review and accepts it in one move, since the human decided while looking at it. The command line MUST NOT
// reach it: --by is self-reported there, so an agent could undo its own withdrawal unseen. Like Accept it resolves
// the finding's open notes and returns their ids, since overriding the withdrawal is the human's answer to them.
func Reinstate(d *Draft, findingID string, now time.Time) ([]string, error) {
	stored, err := findFinding(d, findingID)
	if err != nil {
		return nil, err
	}
	if got := disposition(d, *stored); got != DispositionWithdrawn {
		fix := "accept, exclude or send back the finding instead"
		if !stored.Included {
			// Withdrawn under a current excluded decision. Accept would refuse it too, so restore first: that clears
			// the decision and leaves the finding withdrawn, which reinstate does take.
			fix = fmt.Sprintf("restore %s first, then reinstate it", stored.ID)
		}
		return nil, refusal.New(refusal.Input, fmt.Sprintf("reinstate is for withdrawn findings only; %s is %s", stored.ID, got), fix)
	}
	included := true
	// No publishable field changes, so Edit needs no diff to validate against.
	f, _, err := Edit(d, findingID, EditInput{}, &included, nil, ByHuman, now)
	if err != nil {
		return nil, err
	}
	d.Decisions[f.ID] = Decision{FindingID: f.ID, Decision: DecisionAccepted, FindingRev: f.Rev, At: now}
	return closeOpenNotes(d, f.ID, NoteResolved, now), nil
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

func closeOpenNotes(d *Draft, findingID, status string, now time.Time) []string {
	closed := []string{}
	for i := range d.Notes {
		if n := &d.Notes[i]; n.FindingID == findingID && n.Status == NoteOpen {
			n.Status = status
			n.ClosedAt = &now
			closed = append(closed, n.ID)
		}
	}
	return closed
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
	Verified     json.RawMessage `json:"verified"`
	Impact       json.RawMessage `json:"impact"`
	References   json.RawMessage `json:"references"`
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
	note("verified", next.Verified != stored.Verified, stored.Verified)
	note("impact", next.Impact != stored.Impact, stored.Impact)
	note("references", !slices.Equal(next.References, stored.References), stored.References)
	note("suggestedFix", next.SuggestedFix != stored.SuggestedFix, stored.SuggestedFix)
	publishable := len(changed) > 0
	note("included", next.Included != stored.Included, stored.Included)
	if len(changed) == 0 {
		return *stored, false, ErrNoChange
	}
	if publishable {
		// A severity stored before the enum existed may be free text; it is checked only once an edit changes it.
		severity := next.Severity
		if severity == stored.Severity {
			severity = ""
		}
		err := validateInput(FindingInput{Title: next.Title, Body: next.Body, Location: next.Location, General: next.General, Label: next.Label,
			Blocking: next.Blocking, Confidence: next.Confidence, Severity: severity, Verified: next.Verified, Impact: next.Impact,
			References: next.References, SuggestedFix: next.SuggestedFix}, dif)
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

// Recalibratable refuses what Recalibrate would, so the review interface can refuse before asking for the new values.
func Recalibratable(f Finding) error {
	if f.Included {
		return nil
	}
	return refusal.New(refusal.Input, fmt.Sprintf("%s is withdrawn, so its label and blocking cannot be edited", f.ID),
		fmt.Sprintf("reinstate %s first", f.ID))
}

// Recalibrate is the human's label and blocking edit from the review interface. Unlike Edit it keeps a current
// decision, since the human made the change while looking at the finding. The command line MUST NOT reach it: --by
// is self-reported there, so an agent could change an accepted finding unseen. It closes no note, so a label change
// never answers a note the human has not read.
func Recalibrate(d *Draft, findingID, label string, blocking bool, dif *diff.Diff, now time.Time) (Finding, error) {
	stored, err := findFinding(d, findingID)
	if err != nil {
		return Finding{}, err
	}
	if err := Recalibratable(*stored); err != nil {
		return Finding{}, err
	}
	labelJSON, err := json.Marshal(label)
	if err != nil {
		return Finding{}, err
	}
	blockingJSON, err := json.Marshal(blocking)
	if err != nil {
		return Finding{}, err
	}
	decision, decided := d.Decisions[stored.ID]
	current := decided && decision.FindingRev == stored.Rev
	f, _, err := Edit(d, findingID, EditInput{Label: labelJSON, Blocking: blockingJSON}, nil, dif, ByHuman, now)
	if err != nil {
		return f, err
	}
	if current {
		decision.FindingRev = f.Rev
		d.Decisions[f.ID] = decision
	}
	return f, nil
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
		{"verified", in.Verified, &f.Verified}, {"impact", in.Impact, &f.Impact}, {"suggestedFix", in.SuggestedFix, &f.SuggestedFix}} {
		if err := decodeOptional(field.name, field.raw, field.dst); err != nil {
			return err
		}
	}
	switch {
	case in.References == nil:
	case isNull(in.References):
		f.References = nil
	default:
		var refs []string
		if err := decodeValue("references", in.References, &refs); err != nil {
			return err
		}
		if len(refs) == 0 {
			refs = nil
		}
		f.References = refs
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
		return refusal.New(refusal.Input, fmt.Sprintf("%s cannot be null; only location, label, confidence, severity, verified, impact, references and suggestedFix can be cleared", field), editFix)
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
