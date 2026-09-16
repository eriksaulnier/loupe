package draft

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

var digestNow = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

func digestDraft() *Draft {
	d := NewEmpty()
	d.Summary = "Two <issues> & more"
	d.Findings = []Finding{
		{ID: "f-001", Rev: 1, Title: "T1", Body: "B1", General: true, Label: "question", By: ByAgent, Included: true, CreatedAt: digestNow, UpdatedAt: digestNow, History: []HistoryEntry{}},
		{ID: "f-002", Rev: 1, Title: "T2", Body: "B2\n", Location: &Location{Path: "a.go", Side: SideLeft, Line: 14, StartLine: 10}, Label: "issue", Blocking: true, Confidence: "high", Severity: "minor", SuggestedFix: "fix it", Impact: "Two reviews.", Verified: "reproduced", References: []string{"https://github.com/o/r/issues/1"}, By: ByAgent, Included: true, CreatedAt: digestNow, UpdatedAt: digestNow, History: []HistoryEntry{}},
		{ID: "f-003", Rev: 2, Title: "withdrawn", Body: "gone", General: true, By: ByAgent, Included: false, History: []HistoryEntry{}},
		{ID: "f-004", Rev: 1, Title: "excluded", Body: "no", General: true, By: ByAgent, Included: true, History: []HistoryEntry{}},
	}
	d.Decisions["f-004"] = Decision{FindingID: "f-004", Decision: DecisionExcluded, FindingRev: 1, At: digestNow}
	return d
}

func TestDigestKnownValue(t *testing.T) {
	// sha256 of {"summary":"Two <issues> & more","findings":[{"id":"f-001","title":"T1","body":"B1","location":null,
	// "general":true,"label":"question","blocking":false,"confidence":"","severity":"","suggestedFix":"","impact":"",
	// "verified":"","references":[]},{"id":"f-002","title":"T2","body":"B2\n","location":{"path":"a.go","side":"LEFT",
	// "line":14,"startLine":10},"general":false,"label":"issue","blocking":true,"confidence":"high","severity":"minor",
	// "suggestedFix":"fix it","impact":"Two reviews.","verified":"reproduced","references":["https://github.com/o/r/issues/1"]}]}
	const want = "3a2f60694ab5876ce01fc5cf545a84e9c73f3abac61909a4eb4b2ec6dbe60527"
	if got := Digest(digestDraft()); got != want {
		t.Fatalf("Digest = %s, want %s", got, want)
	}
}

func TestPublishableSet(t *testing.T) {
	d := digestDraft()
	if _, err := Accept(d, "f-002", digestNow); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, f := range PublishableSet(d) {
		ids = append(ids, f.ID)
	}
	if len(ids) != 2 || ids[0] != "f-001" || ids[1] != "f-002" {
		t.Fatalf("PublishableSet ids = %v, want [f-001 f-002]", ids)
	}
}

func TestDigestStable(t *testing.T) {
	base := Digest(digestDraft())
	cases := map[string]func(*Draft){
		"findings reordered": func(d *Draft) {
			d.Findings[0], d.Findings[1], d.Findings[3] = d.Findings[3], d.Findings[0], d.Findings[1]
		},
		"notes and replies": func(d *Draft) {
			d.Notes = append(d.Notes, Note{ID: "n-001", FindingID: "f-001", Body: "why", At: digestNow, Status: NoteOpen})
			d.Replies = append(d.Replies, Reply{ID: "r-001", NoteID: "n-001", Body: "because", By: ByAgent, At: digestNow})
		},
		"history, by and timestamps": func(d *Draft) {
			d.Findings[0].History = append(d.Findings[0].History, HistoryEntry{At: digestNow, By: ByHuman, Changed: map[string]any{"title": "x"}})
			d.Findings[0].By = ByHuman
			d.Findings[0].CreatedAt = digestNow.Add(time.Hour)
			d.Findings[1].UpdatedAt = digestNow.Add(time.Hour)
		},
		"rev and version": func(d *Draft) {
			d.Findings[0].Rev = 7
			d.Version = 9
		},
		"accepting a pending finding": func(d *Draft) {
			if _, err := Accept(d, "f-001", digestNow); err != nil {
				t.Fatal(err)
			}
		},
		"changes to a withdrawn or excluded finding": func(d *Draft) {
			d.Findings[2].Title = "still withdrawn"
			d.Findings[3].Body = "still excluded"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := digestDraft()
			mutate(d)
			if got := Digest(d); got != base {
				t.Fatalf("Digest changed: %s, want %s", got, base)
			}
		})
	}
}

func TestDigestAllPendingEqualsAllAccepted(t *testing.T) {
	d := digestDraft()
	pending := Digest(d)
	for _, id := range []string{"f-001", "f-002"} {
		if _, err := Accept(d, id, digestNow); err != nil {
			t.Fatal(err)
		}
	}
	if got := Digest(d); got != pending {
		t.Fatalf("all accepted %s, all pending %s", got, pending)
	}
}

func TestDigestChanges(t *testing.T) {
	accepted := func(d *Draft) {
		if _, err := Accept(d, "f-002", digestNow); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]struct {
		before func(*Draft)
		change func(*Draft)
	}{
		"summary":       {change: func(d *Draft) { d.Summary += "!" }},
		"pending title": {change: func(d *Draft) { d.Findings[0].Title = "T1b" }},
		"pending body":  {change: func(d *Draft) { d.Findings[0].Body = "B1b" }},
		"pending location": {change: func(d *Draft) {
			d.Findings[0].General, d.Findings[0].Location = false, &Location{Path: "a.go", Side: "RIGHT", Line: 1}
		}},
		"pending label":             {change: func(d *Draft) { d.Findings[0].Label = "" }},
		"pending blocking":          {change: func(d *Draft) { d.Findings[0].Blocking = true }},
		"pending confidence":        {change: func(d *Draft) { d.Findings[0].Confidence = "low" }},
		"pending severity":          {change: func(d *Draft) { d.Findings[0].Severity = "major" }},
		"pending suggested fix":     {change: func(d *Draft) { d.Findings[0].SuggestedFix = "do" }},
		"accepted title":            {before: accepted, change: func(d *Draft) { d.Findings[1].Title = "T2b" }},
		"accepted location line":    {before: accepted, change: func(d *Draft) { d.Findings[1].Location.Line = 13 }},
		"accepted location start":   {before: accepted, change: func(d *Draft) { d.Findings[1].Location.StartLine = 0 }},
		"accepted location side":    {before: accepted, change: func(d *Draft) { d.Findings[1].Location.Side = "RIGHT" }},
		"accepted location path":    {before: accepted, change: func(d *Draft) { d.Findings[1].Location.Path = "b.go" }},
		"accepted finding excluded": {before: accepted, change: func(d *Draft) { must(t, errOf(Exclude(d, "f-002", digestNow))) }},
		"pending finding excluded":  {change: func(d *Draft) { must(t, errOf(Exclude(d, "f-001", digestNow))) }},
		"excluded finding restored": {change: func(d *Draft) { must(t, Restore(d, "f-004")) }},
		"finding withdrawn":         {change: func(d *Draft) { d.Findings[0].Included = false }},
		"finding included":          {change: func(d *Draft) { d.Findings[2].Included = true }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d := digestDraft()
			if c.before != nil {
				c.before(d)
			}
			before := Digest(d)
			c.change(d)
			if Digest(d) == before {
				t.Fatalf("Digest did not change")
			}
		})
	}
}

// errOf drops the closed note ids that Accept and Exclude return.
func errOf(_ []string, err error) error { return err }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestDigestOrdersIDsNumerically(t *testing.T) {
	d := NewEmpty()
	d.Findings = []Finding{
		{ID: "f-1000", Title: "b", Body: "b", General: true, Included: true},
		{ID: "f-999", Title: "a", Body: "a", General: true, Included: true},
	}
	doc := `{"summary":"","findings":[` +
		`{"id":"f-999","title":"a","body":"a","location":null,"general":true,"label":"","blocking":false,"confidence":"","severity":"","suggestedFix":"","impact":"","verified":"","references":[]},` +
		`{"id":"f-1000","title":"b","body":"b","location":null,"general":true,"label":"","blocking":false,"confidence":"","severity":"","suggestedFix":"","impact":"","verified":"","references":[]}]}`
	sum := sha256.Sum256([]byte(doc))
	if got, want := Digest(d), hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("Digest = %s, want %s (f-999 before f-1000)", got, want)
	}
}
