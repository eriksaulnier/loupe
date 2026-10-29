package stats

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/run"
)

var october = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

func fnd(id, label string, blocking bool) draft.Finding {
	return draft.Finding{ID: id, Rev: 1, Title: id, Label: label, Blocking: blocking, By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}}
}

// withdrawn is a finding whose last change of included was by.
func withdrawn(id, by string) draft.Finding {
	f := fnd(id, "issue", false)
	f.Rev, f.Included = 2, false
	f.History = []draft.HistoryEntry{{By: by, Changed: map[string]any{"included": true}}}
	return f
}

func newRun(repo string, number, round int, captured time.Time, findings ...draft.Finding) Run {
	d := draft.NewEmpty()
	d.Version = len(findings) + 1
	d.Findings = findings
	ref := run.Ref{Owner: "example-org", Repo: repo, Number: number, Round: round}
	return Run{Ref: ref, Target: run.Target{Owner: ref.Owner, Repo: repo, Number: number, Round: round, CapturedAt: captured}, Draft: d}
}

func (r Run) decide(decision string, ids ...string) Run {
	for _, id := range ids {
		for _, f := range r.Draft.Findings {
			if f.ID == id {
				r.Draft.Decisions[id] = draft.Decision{FindingID: id, Decision: decision, FindingRev: f.Rev}
			}
		}
	}
	r.Draft.Version++
	return r
}

// publish records a receipt whose envelope holds ids as the draft has them now, posted after the capture.
func (r Run) publish(after time.Duration, viewer string, ids ...string) Run {
	env := publish.Envelope{Viewer: viewer, DraftVersion: r.Draft.Version, Findings: []publish.EnvelopeFinding{}}
	for _, id := range ids {
		for _, f := range r.Draft.Findings {
			if f.ID == id {
				env.Findings = append(env.Findings, publish.EnvelopeFinding{ID: id, Label: f.Label, Blocking: f.Blocking})
			}
		}
	}
	r.Receipt = &publish.Receipt{PostedAt: r.Target.CapturedAt.Add(after), Envelope: env}
	return r
}

func published() Filter { return Filter{States: []string{StatePublished}, GroupBy: GroupNone} }

func ptr[T any](v T) *T { return &v }

// checkFiled is FR-003 over one group.
func checkFiled(t *testing.T, g Group, runs []Run) {
	t.Helper()
	f := g.Findings
	if sum := f.Published + f.Excluded + f.Withdrawn + f.Pending; sum != f.Filed {
		t.Errorf("%s: published %d + excluded %d + withdrawn %d + pending %d = %d, want filed %d",
			g.Key, f.Published, f.Excluded, f.Withdrawn, f.Pending, sum, f.Filed)
	}
	if runs == nil {
		return
	}
	n := 0
	for _, r := range runs {
		n += len(r.Draft.Findings)
	}
	if f.Filed != n {
		t.Errorf("%s: filed %d, want the %d findings of the covered drafts", g.Key, f.Filed, n)
	}
}

func TestPublishedFindingsCountFromTheReceipt(t *testing.T) {
	r := newRun("one", 1, 1, october, fnd("f-001", "issue", true), fnd("f-002", "question", false)).
		decide(draft.DecisionAccepted, "f-001", "f-002").publish(10*time.Minute, "reviewer", "f-001", "f-002")
	// Relabeled after publication; the review showed the envelope's label.
	r.Draft.Findings[0].Label, r.Draft.Findings[0].Blocking = "suggestion", false
	got := Compute([]Run{r}, published()).Total
	if got.Findings.Published != 2 || got.ByLabel.Published != (LabelCounts{Issue: 1, Question: 1}) || got.Blocking.Published != 1 {
		t.Fatalf("published %d, byLabel %+v, blocking %d; want 2, one issue and one question, 1 blocking",
			got.Findings.Published, got.ByLabel.Published, got.Blocking.Published)
	}
	checkFiled(t, got, []Run{r})
}

func TestHumanWithdrawalCountsAsExcludedAndAgentWithdrawalAsWithdrawn(t *testing.T) {
	r := newRun("one", 1, 1, october,
		fnd("f-001", "issue", false), fnd("f-002", "suggestion", true), withdrawn("f-003", draft.ByHuman),
		withdrawn("f-004", draft.ByAgent), withdrawn("f-005", draft.ByAgent)).
		decide(draft.DecisionAccepted, "f-001").decide(draft.DecisionExcluded, "f-002").publish(time.Minute, "reviewer", "f-001")
	got := Compute([]Run{r}, published()).Total
	want := Findings{Filed: 5, Published: 1, Excluded: 2, Withdrawn: 2}
	if got.Findings != want {
		t.Fatalf("findings %+v, want %+v", got.Findings, want)
	}
	// One published of three gated: the agent's two withdrawals are not in the divisor.
	if got.Precision == nil || *got.Precision != 0.333 {
		t.Fatalf("precision %v, want 0.333", got.Precision)
	}
	if got.ByLabel.Excluded != (LabelCounts{Issue: 1, Suggestion: 1}) || got.Blocking.Excluded != 1 {
		t.Fatalf("excluded byLabel %+v, blocking %d; want one issue, one suggestion, 1 blocking", got.ByLabel.Excluded, got.Blocking.Excluded)
	}
}

func TestExcludedBreakdownUsesTheCurrentLabelAndGroupsUnknownLabelsAsOther(t *testing.T) {
	regraded := fnd("f-001", "issue", false)
	regraded.Rev, regraded.Label, regraded.Blocking = 2, "perf-nit", true
	regraded.History = []draft.HistoryEntry{{By: draft.ByHuman, Changed: map[string]any{"label": "issue", "blocking": false}}}
	r := newRun("one", 1, 1, october, regraded, fnd("f-002", "", false)).
		decide(draft.DecisionExcluded, "f-001", "f-002").publish(time.Minute, "reviewer")
	got := Compute([]Run{r}, published()).Total
	if got.ByLabel.Excluded != (LabelCounts{Other: 2}) || got.Blocking.Excluded != 1 {
		t.Fatalf("excluded byLabel %+v, blocking %d; want two other, 1 blocking", got.ByLabel.Excluded, got.Blocking.Excluded)
	}
	if got.Findings.Regraded != 1 {
		t.Fatalf("regraded %d, want 1", got.Findings.Regraded)
	}
}

func TestReinstatedAndRegradedAreGateOfsFlags(t *testing.T) {
	reinstated := fnd("f-001", "issue", false)
	reinstated.Rev = 3
	reinstated.History = []draft.HistoryEntry{
		{By: draft.ByAgent, Changed: map[string]any{"included": true}},
		{By: draft.ByHuman, Changed: map[string]any{"included": false}},
	}
	regraded := fnd("f-002", "issue", false)
	regraded.Rev = 2
	regraded.History = []draft.HistoryEntry{{By: draft.ByHuman, Changed: map[string]any{"severity": "low"}}}
	r := newRun("one", 1, 1, october, reinstated, regraded).
		decide(draft.DecisionAccepted, "f-001", "f-002").publish(time.Minute, "reviewer", "f-001", "f-002")
	got := Compute([]Run{r}, published()).Total.Findings
	if got.Reinstated != 1 || got.Regraded != 1 || got.Published != 2 {
		t.Fatalf("findings %+v, want 2 published, 1 reinstated, 1 regraded", got)
	}
}

func TestUnattendedRunCountsItsPendingFindingsAsPublished(t *testing.T) {
	r := newRun("one", 1, 1, october, fnd("f-001", "issue", false), fnd("f-002", "suggestion", false)).
		publish(time.Minute, "", "f-001", "f-002")
	got := Compute([]Run{r}, published()).Total
	want := Findings{Filed: 2, Published: 2}
	if got.Findings != want || got.Runs != (Runs{Published: 1, Unattended: 1}) {
		t.Fatalf("findings %+v, runs %+v; want %+v and one unattended published run", got.Findings, got.Runs, want)
	}
}

func TestPrecisionAndMedianLeaveOutUnattendedRuns(t *testing.T) {
	attended := newRun("one", 1, 1, october,
		fnd("f-001", "issue", false), fnd("f-002", "issue", false), fnd("f-003", "issue", false), fnd("f-004", "issue", false)).
		decide(draft.DecisionAccepted, "f-001", "f-002", "f-003").decide(draft.DecisionExcluded, "f-004").
		publish(14*time.Minute, "reviewer", "f-001", "f-002", "f-003")
	var five []draft.Finding
	for _, id := range []string{"f-001", "f-002", "f-003", "f-004", "f-005"} {
		five = append(five, fnd(id, "issue", false))
	}
	unattended := newRun("one", 2, 1, october, five...).publish(3*time.Hour, "", "f-001", "f-002", "f-003", "f-004", "f-005")
	got := Compute([]Run{attended, unattended}, published()).Total
	if got.Findings.Published != 8 || got.Runs.Unattended != 1 {
		t.Fatalf("published %d, unattended %d; want 8 and 1", got.Findings.Published, got.Runs.Unattended)
	}
	if got.Precision == nil || *got.Precision != 0.75 {
		t.Fatalf("precision %v, want 0.75", got.Precision)
	}
	if got.MedianCaptureToPublish == nil || got.MedianCaptureToPublish.String() != "PT14M" {
		t.Fatalf("median %v, want PT14M", got.MedianCaptureToPublish)
	}

	only := Compute([]Run{unattended}, published()).Total
	if only.Precision != nil || only.MedianCaptureToPublish != nil {
		t.Fatalf("precision %v, median %v over unattended runs only; want both nil", only.Precision, only.MedianCaptureToPublish)
	}
}

func TestPrecisionRoundsAndIsNilWithNothingGated(t *testing.T) {
	r := newRun("one", 1, 1, october, fnd("f-001", "issue", false), fnd("f-002", "issue", false), fnd("f-003", "issue", false)).
		decide(draft.DecisionAccepted, "f-001", "f-002").decide(draft.DecisionExcluded, "f-003").
		publish(time.Minute, "reviewer", "f-001", "f-002")
	if got := Compute([]Run{r}, published()).Total.Precision; got == nil || *got != 0.667 {
		t.Fatalf("precision %v, want 0.667", got)
	}
	empty := newRun("one", 2, 1, october, withdrawn("f-001", draft.ByAgent)).publish(time.Minute, "reviewer")
	if got := Compute([]Run{empty}, published()).Total.Precision; got != nil {
		t.Fatalf("precision %v with nothing published or excluded, want nil", *got)
	}
}

func TestDriftedRunIsListedAndStillCounts(t *testing.T) {
	r := newRun("one", 7, 2, october, fnd("f-001", "issue", false), fnd("f-002", "issue", false)).
		decide(draft.DecisionAccepted, "f-001", "f-002").publish(time.Minute, "reviewer", "f-001", "f-002")
	// After publication: f-002 excluded, and f-003 added and accepted.
	r.Draft.Findings = append(r.Draft.Findings, fnd("f-003", "question", false))
	r = r.decide(draft.DecisionExcluded, "f-002").decide(draft.DecisionAccepted, "f-003")
	clean := newRun("one", 8, 1, october, fnd("f-001", "issue", false)).
		decide(draft.DecisionAccepted, "f-001").publish(time.Minute, "reviewer", "f-001")
	got := Compute([]Run{r, clean}, published()).Total
	if !reflect.DeepEqual(got.Drifted, []string{"example-org/one#7@2"}) {
		t.Fatalf("drifted %v, want [example-org/one#7@2]", got.Drifted)
	}
	want := Findings{Filed: 4, Published: 3, Pending: 1}
	if got.Findings != want {
		t.Fatalf("findings %+v, want %+v", got.Findings, want)
	}
	checkFiled(t, got, []Run{r, clean})
}

func TestUnpublishedRunsCountNothingPublished(t *testing.T) {
	ready := newRun("one", 1, 1, october, fnd("f-001", "issue", false), withdrawn("f-002", draft.ByAgent)).
		decide(draft.DecisionAccepted, "f-001")
	captured := newRun("one", 2, 1, october, fnd("f-001", "issue", false))
	all := Filter{States: []string{StateCaptured, StateReady, StatePublished}, GroupBy: GroupNone}
	got := Compute([]Run{ready, captured}, all).Total
	if got.Runs != (Runs{Captured: 1, Ready: 1}) {
		t.Fatalf("runs %+v, want one captured and one ready", got.Runs)
	}
	want := Findings{Filed: 3, Withdrawn: 1, Pending: 2}
	if got.Findings != want || got.ByLabel.Published != (LabelCounts{}) || got.Blocking.Published != 0 {
		t.Fatalf("findings %+v, published byLabel %+v; want %+v and nothing published", got.Findings, got.ByLabel.Published, want)
	}
	if got := Compute([]Run{ready, captured}, published()).Total.Runs; got != (Runs{}) {
		t.Fatalf("the default filter covered %+v, want no runs", got)
	}
}

func TestRoundsAreRunsAndPullRequestsAreDistinct(t *testing.T) {
	first := newRun("one", 5, 1, october, fnd("f-001", "issue", false)).decide(draft.DecisionAccepted, "f-001").
		publish(time.Minute, "reviewer", "f-001")
	second := newRun("one", 5, 2, october.Add(time.Hour), fnd("f-001", "issue", false)).decide(draft.DecisionAccepted, "f-001").
		publish(time.Minute, "reviewer", "f-001")
	got := Compute([]Run{first, second}, published()).Total
	if got.Runs.Published != 2 || got.PullRequests != 1 {
		t.Fatalf("runs %d, pull requests %d; want 2 and 1", got.Runs.Published, got.PullRequests)
	}
}

func TestMedianCaptureToPublish(t *testing.T) {
	cases := []struct {
		name  string
		after []time.Duration
		want  string
	}{
		{"one", []time.Duration{14 * time.Minute}, "PT14M"},
		{"even takes the mean of the middle two", []time.Duration{time.Hour, time.Hour + 4*time.Minute + 10*time.Second, 9 * time.Hour, 0}, "PT1H2M5S"},
		{"odd takes the middle", []time.Duration{3 * time.Second, 26 * time.Hour, 25 * time.Hour}, "PT25H"},
		{"zero", []time.Duration{0}, "PT0S"},
		{"rounds to the second", []time.Duration{time.Second, 2 * time.Second}, "PT2S"},
		{"hours stay hours", []time.Duration{26 * time.Hour}, "PT26H"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var runs []Run
			for i, after := range c.after {
				runs = append(runs, newRun("one", i+1, 1, october).publish(after, "reviewer"))
			}
			got := Compute(runs, published()).Total.MedianCaptureToPublish
			if got == nil || got.String() != c.want {
				t.Fatalf("median %v, want %s", got, c.want)
			}
		})
	}
}

func TestEmptySelectionIsZeroWithNullsAndEmptyLists(t *testing.T) {
	got := Compute(nil, Filter{States: []string{StatePublished}, GroupBy: GroupRepo})
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"groups":[],"total":{"key":"total","runs":{"captured":0,"ready":0,"published":0,"unattended":0},"pullRequests":0,` +
		`"findings":{"filed":0,"published":0,"excluded":0,"withdrawn":0,"pending":0,"reinstated":0,"regraded":0},` +
		`"byLabel":{"published":{"issue":0,"suggestion":0,"question":0,"other":0},"excluded":{"issue":0,"suggestion":0,"question":0,"other":0}},` +
		`"blocking":{"published":0,"excluded":0},"precision":null,"drifted":[],"medianCaptureToPublish":null}}`
	if string(b) != want {
		t.Fatalf("got\n%s\nwant\n%s", b, want)
	}
}

func TestDurationJSONIsAnISO8601String(t *testing.T) {
	b, err := json.Marshal(ptr(Duration(time.Hour + 2*time.Minute + 5*time.Second)))
	if err != nil || string(b) != `"PT1H2M5S"` {
		t.Fatalf("got %s, %v; want \"PT1H2M5S\"", b, err)
	}
	if got := Duration(-5 * time.Second).String(); got != "-PT5S" {
		t.Fatalf("negative duration %q, want -PT5S", got)
	}
}
