// Package stats rolls the human gate up across runs: what the agent filed, what was published, what the human
// rejected and what the agent cut itself, counted the way the published loupe-meta marker counts them.
package stats

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/section"
)

const (
	StateCaptured  = "captured"
	StateReady     = "ready"
	StatePublished = "published"
)

// States is every state in the order a filter echoes them.
var States = []string{StateCaptured, StateReady, StatePublished}

const (
	GroupNone  = "none"
	GroupRepo  = "repo"
	GroupWeek  = "week"
	GroupMonth = "month"
)

var GroupBys = []string{GroupNone, GroupRepo, GroupWeek, GroupMonth}

// Run is one round as its files hold it. Receipt is nil when the round is not published.
type Run struct {
	Ref     run.Ref
	Target  run.Target
	Draft   *draft.Draft
	Receipt *publish.Receipt
}

type Filter struct {
	// Repos are owner/repo, matched ignoring case; none matches every repository.
	Repos []string
	// Since and Until are inclusive bounds on a run's time; nil leaves that side open.
	Since, Until *time.Time
	States       []string
	GroupBy      string
}

type Result struct {
	Groups []Group `json:"groups"`
	Total  Group   `json:"total"`
}

type Group struct {
	Key          string   `json:"key"`
	Runs         Runs     `json:"runs"`
	PullRequests int      `json:"pullRequests"`
	Findings     Findings `json:"findings"`
	ByLabel      ByLabel  `json:"byLabel"`
	Blocking     Blocking `json:"blocking"`
	// Precision and MedianCaptureToPublish are over attended published runs only, nil when there is nothing to divide.
	Precision              *float64  `json:"precision"`
	Drifted                []string  `json:"drifted"`
	MedianCaptureToPublish *Duration `json:"medianCaptureToPublish"`
	// Gate stays out of the JSON, which carries only the ratio; the human output prints it beside precision.
	Gate Gate `json:"-"`
}

// Gate is precision's dividend and divisor: published, and published plus excluded, over attended published runs.
type Gate struct {
	Published, Gated int
}

type Runs struct {
	Captured  int `json:"captured"`
	Ready     int `json:"ready"`
	Published int `json:"published"`
	// Unattended is a subset of Published.
	Unattended int `json:"unattended"`
}

// Findings splits Filed into Published, Excluded, Withdrawn and Pending. Reinstated and Regraded are flags that can
// overlap any of them.
type Findings struct {
	Filed      int `json:"filed"`
	Published  int `json:"published"`
	Excluded   int `json:"excluded"`
	Withdrawn  int `json:"withdrawn"`
	Pending    int `json:"pending"`
	Reinstated int `json:"reinstated"`
	Regraded   int `json:"regraded"`
}

type ByLabel struct {
	Published LabelCounts `json:"published"`
	Excluded  LabelCounts `json:"excluded"`
}

type LabelCounts struct {
	Issue      int `json:"issue"`
	Suggestion int `json:"suggestion"`
	Question   int `json:"question"`
	Other      int `json:"other"`
}

func (c *LabelCounts) add(label string) {
	switch section.Group(label) {
	case section.Issue:
		c.Issue++
	case section.Suggestion:
		c.Suggestion++
	case section.Question:
		c.Question++
	default:
		c.Other++
	}
}

type Blocking struct {
	Published int `json:"published"`
	Excluded  int `json:"excluded"`
}

// Duration is an ISO 8601 duration in whole seconds, in hours, minutes and seconds only, since a day there is a
// calendar day rather than 24 hours.
type Duration time.Duration

func (d Duration) String() string {
	s := time.Duration(d).Round(time.Second)
	sign := ""
	if s < 0 {
		sign, s = "-", -s
	}
	h, m, sec := int64(s/time.Hour), int64(s%time.Hour/time.Minute), int64(s%time.Minute/time.Second)
	var b strings.Builder
	b.WriteString(sign + "PT")
	if h > 0 {
		fmt.Fprintf(&b, "%dH", h)
	}
	if m > 0 {
		fmt.Fprintf(&b, "%dM", m)
	}
	if sec > 0 || h == 0 && m == 0 {
		fmt.Fprintf(&b, "%dS", sec)
	}
	return b.String()
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// State is the one loupe list gives the run.
func State(r Run) string {
	switch {
	case r.Receipt != nil:
		return StatePublished
	case draft.ReadinessOf(r.Draft).Ready:
		return StateReady
	}
	return StateCaptured
}

// Time is when the run was published, or captured when it is not.
func Time(r Run) time.Time {
	if r.Receipt != nil {
		return r.Receipt.PostedAt.UTC()
	}
	return r.Target.CapturedAt.UTC()
}

func Compute(runs []Run, f Filter) Result {
	total := newAccumulator("total")
	groups := map[string]*accumulator{}
	for _, r := range runs {
		if !f.matches(r) {
			continue
		}
		total.add(r)
		if f.GroupBy == GroupNone {
			continue
		}
		key := groupKey(r, f.GroupBy)
		if groups[key] == nil {
			groups[key] = newAccumulator(key)
		}
		groups[key].add(r)
	}
	out := Result{Groups: []Group{}, Total: total.group()}
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		out.Groups = append(out.Groups, groups[key].group())
	}
	return out
}

func (f Filter) matches(r Run) bool {
	if !slices.Contains(f.States, State(r)) {
		return false
	}
	if len(f.Repos) > 0 && !slices.ContainsFunc(f.Repos, func(repo string) bool {
		return strings.EqualFold(repo, r.Ref.Owner+"/"+r.Ref.Repo)
	}) {
		return false
	}
	t := Time(r)
	return (f.Since == nil || !t.Before(*f.Since)) && (f.Until == nil || !t.After(*f.Until))
}

// groupKey sorts as a string in the order of its group: week and month keys are zero-padded.
func groupKey(r Run, by string) string {
	t := Time(r)
	switch by {
	case GroupWeek:
		year, week := t.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", year, week)
	case GroupMonth:
		return t.Format("2006-01")
	}
	return r.Ref.Owner + "/" + r.Ref.Repo
}

type accumulator struct {
	g Group
	// pullRequests holds owner/repo#n.
	pullRequests map[string]bool
	durations    []time.Duration
}

func newAccumulator(key string) *accumulator {
	return &accumulator{g: Group{Key: key, Drifted: []string{}}, pullRequests: map[string]bool{}}
}

func (a *accumulator) add(r Run) {
	switch State(r) {
	case StateCaptured:
		a.g.Runs.Captured++
	case StateReady:
		a.g.Runs.Ready++
	case StatePublished:
		a.g.Runs.Published++
	}
	a.pullRequests[fmt.Sprintf("%s/%s#%d", r.Ref.Owner, r.Ref.Repo, r.Ref.Number)] = true

	sent := map[string]publish.EnvelopeFinding{}
	attended := false
	if r.Receipt != nil {
		for _, ef := range r.Receipt.Envelope.Findings {
			sent[ef.ID] = ef
		}
		if r.Draft.Version != r.Receipt.Envelope.DraftVersion {
			a.g.Drifted = append(a.g.Drifted, r.Ref.String())
		}
		if r.Receipt.Envelope.Unattended() {
			a.g.Runs.Unattended++
		} else {
			attended = true
			a.durations = append(a.durations, r.Receipt.PostedAt.Sub(r.Target.CapturedAt))
		}
	}

	var published, excluded int
	for _, f := range r.Draft.Findings {
		a.g.Findings.Filed++
		gate := draft.GateOf(r.Draft, f)
		if gate.Reinstated {
			a.g.Findings.Reinstated++
		}
		if gate.Regraded {
			a.g.Findings.Regraded++
		}
		// The receipt is what the review showed, whatever the draft says since.
		if ef, ok := sent[f.ID]; ok {
			published++
			a.g.ByLabel.Published.add(ef.Label)
			if ef.Blocking {
				a.g.Blocking.Published++
			}
			continue
		}
		switch gate.Disposition {
		case draft.DispositionExcluded:
			excluded++
			a.g.ByLabel.Excluded.add(f.Label)
			if f.Blocking {
				a.g.Blocking.Excluded++
			}
		case draft.DispositionWithdrawn:
			a.g.Findings.Withdrawn++
		default:
			// Accepted or pending: still in the publishable set and not yet sent.
			a.g.Findings.Pending++
		}
	}
	a.g.Findings.Published += published
	a.g.Findings.Excluded += excluded
	if attended {
		a.g.Gate.Published += published
		a.g.Gate.Gated += published + excluded
	}
}

func (a *accumulator) group() Group {
	g := a.g
	g.PullRequests = len(a.pullRequests)
	if g.Gate.Gated > 0 {
		p := math.Round(float64(g.Gate.Published)/float64(g.Gate.Gated)*1000) / 1000
		g.Precision = &p
	}
	if n := len(a.durations); n > 0 {
		d := slices.Clone(a.durations)
		slices.Sort(d)
		m := d[n/2]
		if n%2 == 0 {
			m = d[n/2-1] + (d[n/2]-d[n/2-1])/2
		}
		median := Duration(m.Round(time.Second))
		g.MedianCaptureToPublish = &median
	}
	return g
}
