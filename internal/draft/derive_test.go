package draft

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/run"
)

func finding(id string, rev int, included bool, by string) Finding {
	return Finding{ID: id, Rev: rev, Title: id, Body: "b", General: true, By: by, Included: included, History: []HistoryEntry{}}
}

func TestDerive(t *testing.T) {
	cases := []struct {
		name         string
		findings     []Finding
		decisions    map[string]Decision
		notes        []Note
		dispositions map[string]string
		readiness    Readiness
		included     int
	}{
		{
			name:         "empty draft is ready",
			dispositions: map[string]string{},
			readiness:    Readiness{Ready: true, Accepted: []string{}, Pending: []string{}, Excluded: []string{}, Withdrawn: []string{}, OpenNotes: []string{}},
		},
		{
			name:         "stale decision is ignored",
			findings:     []Finding{finding("f-001", 2, true, ByAgent)},
			decisions:    map[string]Decision{"f-001": {FindingID: "f-001", Decision: DecisionAccepted, FindingRev: 1}},
			dispositions: map[string]string{"f-001": DispositionPending},
			readiness:    Readiness{Accepted: []string{}, Pending: []string{"f-001"}, Excluded: []string{}, Withdrawn: []string{}, OpenNotes: []string{}},
			included:     1,
		},
		{
			name:     "excluded finding is still included",
			findings: []Finding{finding("f-001", 1, true, ByAgent), finding("f-002", 1, true, ByAgent)},
			decisions: map[string]Decision{
				"f-001": {FindingID: "f-001", Decision: DecisionExcluded, FindingRev: 1},
				"f-002": {FindingID: "f-002", Decision: DecisionAccepted, FindingRev: 1},
			},
			dispositions: map[string]string{"f-001": DispositionExcluded, "f-002": DispositionAccepted},
			readiness:    Readiness{Ready: true, Accepted: []string{"f-002"}, Pending: []string{}, Excluded: []string{"f-001"}, Withdrawn: []string{}, OpenNotes: []string{}},
			included:     2,
		},
		{
			name:         "withdrawn finding does not block readiness",
			findings:     []Finding{finding("f-001", 2, false, ByAgent)},
			dispositions: map[string]string{"f-001": DispositionWithdrawn},
			readiness:    Readiness{Ready: true, Accepted: []string{}, Pending: []string{}, Excluded: []string{}, Withdrawn: []string{"f-001"}, OpenNotes: []string{}},
		},
		{
			name:      "open note blocks readiness",
			findings:  []Finding{finding("f-001", 1, true, ByAgent)},
			decisions: map[string]Decision{"f-001": {FindingID: "f-001", Decision: DecisionAccepted, FindingRev: 1}},
			notes: []Note{
				{ID: "n-001", FindingID: "f-001", Status: NoteResolved},
				{ID: "n-002", FindingID: "f-001", Status: NoteOpen},
				{ID: "n-003", FindingID: "f-001", Status: NoteDismissed},
			},
			dispositions: map[string]string{"f-001": DispositionAccepted},
			readiness:    Readiness{Accepted: []string{"f-001"}, Pending: []string{}, Excluded: []string{}, Withdrawn: []string{}, OpenNotes: []string{"n-002"}},
			included:     1,
		},
		{
			name:         "finding by a human is still pending",
			findings:     []Finding{finding("f-001", 1, true, ByHuman)},
			dispositions: map[string]string{"f-001": DispositionPending},
			readiness:    Readiness{Accepted: []string{}, Pending: []string{"f-001"}, Excluded: []string{}, Withdrawn: []string{}, OpenNotes: []string{}},
			included:     1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewEmpty()
			d.Findings = append(d.Findings, c.findings...)
			for k, v := range c.decisions {
				d.Decisions[k] = v
			}
			d.Notes = append(d.Notes, c.notes...)
			if got := Dispositions(d); !reflect.DeepEqual(got, c.dispositions) {
				t.Fatalf("dispositions %v, want %v", got, c.dispositions)
			}
			if got := ReadinessOf(d); !reflect.DeepEqual(got, c.readiness) {
				t.Fatalf("readiness %+v, want %+v", got, c.readiness)
			}
			if got := IncludedCount(d); got != c.included {
				t.Fatalf("included %d, want %d", got, c.included)
			}
		})
	}
}

func TestFindingStateCoversWhatADecisionRestsOn(t *testing.T) {
	dir := t.TempDir()
	d := NewEmpty()
	if _, err := Add(d, []FindingInput{
		{Title: "One", Body: "Body.", General: true, Label: "question"},
		{Title: "Two", Body: "Body.", General: true, Label: "question"},
	}, nil, ByAgent, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	state := func(d *Draft) string {
		t.Helper()
		b, err := FindingState(d, "f-001")
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	mutate := func(fn func(*Draft) error) *Draft {
		t.Helper()
		d, err := Mutate(dir, "test", nil, noEnv, fn)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	shown := state(loadStored(t, dir))

	// time.Now carries a monotonic reading that a reload drops; the state must not see the difference.
	returned := mutate(func(d *Draft) error { d.Summary = "Elsewhere."; return nil })
	if state(returned) != shown || state(loadStored(t, dir)) != shown {
		t.Fatal("a write elsewhere, or a reload, changed the finding's state")
	}
	mutate(func(d *Draft) error {
		_, err := Add(d, []FindingInput{{Title: "Three", Body: "Body.", General: true, Label: "question"}}, nil, ByAgent, time.Now())
		return err
	})
	mutate(func(d *Draft) error { _, err := Accept(d, "f-002", time.Now()); return err })
	if state(loadStored(t, dir)) != shown {
		t.Fatal("a new finding or a decision on another finding changed the state")
	}

	title, _ := json.Marshal("One, retitled")
	// Each step builds on the one before it: the reply needs the note the send-back wrote.
	for _, step := range []struct {
		name string
		fn   func(*Draft) error
	}{
		{"accept", func(d *Draft) error { _, err := Accept(d, "f-001", time.Now()); return err }},
		{"send back", func(d *Draft) error { _, err := SendBack(d, "f-001", "Why?", time.Now()); return err }},
		{"reply", func(d *Draft) error { _, err := AddReply(d, "n-001", "Because.", ByAgent, time.Now()); return err }},
		{"resolve", func(d *Draft) error { return ResolveNote(d, "n-001", time.Now()) }},
		{"edit", func(d *Draft) error {
			_, _, err := Edit(d, "f-001", EditInput{Title: title}, nil, nil, ByAgent, time.Now())
			return err
		}},
		{"withdraw", func(d *Draft) error {
			excluded := false
			_, _, err := Edit(d, "f-001", EditInput{}, &excluded, nil, ByAgent, time.Now())
			return err
		}},
	} {
		before := state(loadStored(t, dir))
		mutate(step.fn)
		if state(loadStored(t, dir)) == before {
			t.Errorf("%s did not change the finding's state", step.name)
		}
	}
}

// Worth a look is one section, so severity outranks the label there: a critical question comes before a minor
// issue. A blocking finding still leads, whatever its severity.
func TestOrderedSortsSeverityAcrossLabels(t *testing.T) {
	f := func(id, label, sev string, blocking bool) Finding {
		x := finding(id, 1, true, "agent")
		x.Label, x.Severity, x.Blocking = label, sev, blocking
		return x
	}
	d := &Draft{Findings: []Finding{
		f("f-001", "issue", "minor", false),
		f("f-002", "question", "critical", false),
		f("f-003", "suggestion", "minor", true),
		f("f-004", "suggestion", "minor", false),
	}}
	var got []string
	for _, x := range Ordered(d) {
		got = append(got, x.ID)
	}
	if want := []string{"f-003", "f-002", "f-001", "f-004"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Ordered = %v, want %v", got, want)
	}
}

func TestGateCountsOf(t *testing.T) {
	entry := func(by string, changed map[string]any) HistoryEntry {
		return HistoryEntry{By: by, Changed: changed}
	}
	withdraw := func(by string) HistoryEntry { return entry(by, map[string]any{"included": true}) }
	reinstate := func(by string) HistoryEntry { return entry(by, map[string]any{"included": false}) }
	accept := func(rev int) *Decision {
		return &Decision{FindingID: "f-001", Decision: DecisionAccepted, FindingRev: rev}
	}
	exclude := func(rev int) *Decision {
		return &Decision{FindingID: "f-001", Decision: DecisionExcluded, FindingRev: rev}
	}

	cases := []struct {
		name     string
		finding  Finding
		decision *Decision
		want     GateCounts
	}{
		{
			name:     "withdrawn by the agent, then reinstated and accepted",
			finding:  withHistory(finding("f-001", 3, true, ByAgent), withdraw(ByAgent), reinstate(ByHuman)),
			decision: accept(3),
			want:     GateCounts{Reinstated: 1},
		},
		{
			name:    "reinstated, then withdrawn by the agent",
			finding: withHistory(finding("f-001", 4, false, ByAgent), withdraw(ByAgent), reinstate(ByHuman), withdraw(ByAgent)),
			want:    GateCounts{Withdrawn: 1, Reinstated: 1},
		},
		{
			name:    "reinstated, then withdrawn by a human",
			finding: withHistory(finding("f-001", 4, false, ByAgent), withdraw(ByAgent), reinstate(ByHuman), withdraw(ByHuman)),
			want:    GateCounts{Excluded: 1, Reinstated: 1},
		},
		{
			name:    "withdrawn by a human is excluded, not reinstated",
			finding: withHistory(finding("f-001", 2, false, ByAgent), withdraw(ByHuman)),
			want:    GateCounts{Excluded: 1},
		},
		{
			name:    "withdrawn by a human, then included and withdrawn again by the agent",
			finding: withHistory(finding("f-001", 4, false, ByAgent), withdraw(ByHuman), reinstate(ByAgent), withdraw(ByAgent)),
			want:    GateCounts{Withdrawn: 1},
		},
		{
			name:     "withdrawn by the agent, then excluded",
			finding:  withHistory(finding("f-001", 2, false, ByAgent), withdraw(ByAgent)),
			decision: exclude(2),
			want:     GateCounts{Excluded: 1},
		},
		{
			name:     "relabeled by a human, then excluded",
			finding:  withHistory(finding("f-001", 2, true, ByAgent), entry(ByHuman, map[string]any{"label": "issue"})),
			decision: exclude(2),
			want:     GateCounts{Excluded: 1, Regraded: 1},
		},
		{
			name:     "blocking changed by a human",
			finding:  withHistory(finding("f-001", 2, true, ByAgent), entry(ByHuman, map[string]any{"blocking": true})),
			decision: accept(2),
			want:     GateCounts{Regraded: 1},
		},
		{
			name:     "severity changed by a human",
			finding:  withHistory(finding("f-001", 2, true, ByAgent), entry(ByHuman, map[string]any{"severity": "low"})),
			decision: accept(2),
			want:     GateCounts{Regraded: 1},
		},
		{
			name: "blocking and severity changed by a human count once",
			finding: withHistory(finding("f-001", 3, true, ByAgent),
				entry(ByHuman, map[string]any{"blocking": true}), entry(ByHuman, map[string]any{"severity": "low", "label": "issue"})),
			decision: accept(3),
			want:     GateCounts{Regraded: 1},
		},
		{
			name:     "relabeled by the agent",
			finding:  withHistory(finding("f-001", 2, true, ByAgent), entry(ByAgent, map[string]any{"label": "issue"})),
			decision: accept(2),
			want:     GateCounts{},
		},
		{
			name:     "a stale exclusion on an included finding counts for nothing",
			finding:  withHistory(finding("f-001", 2, true, ByAgent), entry(ByAgent, map[string]any{"title": "old"})),
			decision: exclude(1),
			want:     GateCounts{},
		},
		{
			name:     "a stale exclusion on a finding a human withdrew counts by the withdrawal rule",
			finding:  withHistory(finding("f-001", 2, false, ByAgent), withdraw(ByHuman)),
			decision: exclude(1),
			want:     GateCounts{Excluded: 1},
		},
		{
			name:     "a current accept on a finding the agent withdrew",
			finding:  withHistory(finding("f-001", 2, false, ByAgent), withdraw(ByAgent)),
			decision: accept(2),
			want:     GateCounts{Withdrawn: 1},
		},
		{
			name:     "a current exclusion on a finding a human withdrew counts once",
			finding:  withHistory(finding("f-001", 2, false, ByAgent), withdraw(ByHuman)),
			decision: exclude(2),
			want:     GateCounts{Excluded: 1},
		},
		{
			name:    "not included with no history",
			finding: finding("f-001", 1, false, ByAgent),
			want:    GateCounts{Withdrawn: 1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewEmpty()
			d.Findings = append(d.Findings, c.finding)
			if c.decision != nil {
				d.Decisions["f-001"] = *c.decision
			}
			if got := GateCountsOf(roundTrip(t, d)); got != c.want {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

// The filed identity: every finding either publishes or counts in exactly one of Excluded and Withdrawn.
func TestGateCountsPartitionTheFiledFindings(t *testing.T) {
	d := NewEmpty()
	d.Findings = []Finding{
		finding("f-001", 1, true, ByAgent),
		finding("f-002", 1, true, ByAgent),
		withHistory(finding("f-003", 2, false, ByAgent), HistoryEntry{By: ByAgent, Changed: map[string]any{"included": true}}),
		withHistory(finding("f-004", 2, false, ByAgent), HistoryEntry{By: ByHuman, Changed: map[string]any{"included": true}}),
		finding("f-005", 1, true, ByAgent),
	}
	d.Decisions["f-001"] = Decision{FindingID: "f-001", Decision: DecisionAccepted, FindingRev: 1}
	d.Decisions["f-002"] = Decision{FindingID: "f-002", Decision: DecisionExcluded, FindingRev: 1}
	r, g := ReadinessOf(d), GateCountsOf(roundTrip(t, d))
	if got := len(r.Accepted) + len(r.Pending) + g.Excluded + g.Withdrawn; got != len(d.Findings) {
		t.Fatalf("accepted %d + pending %d + excluded %d + withdrawn %d = %d, want %d",
			len(r.Accepted), len(r.Pending), g.Excluded, g.Withdrawn, got, len(d.Findings))
	}
	if want := (GateCounts{Excluded: 2, Withdrawn: 1}); g != want {
		t.Fatalf("got %+v, want %+v", g, want)
	}
}

func withHistory(f Finding, history ...HistoryEntry) Finding {
	f.History = history
	return f
}

// roundTrip passes the draft through its stored JSON, so history values are what a loaded draft holds.
func roundTrip(t *testing.T, d *Draft) *Draft {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var out Draft
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return &out
}
