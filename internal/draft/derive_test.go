package draft

import (
	"reflect"
	"testing"
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
