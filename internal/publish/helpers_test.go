package publish

import (
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

var fixtureNow = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

const (
	headSHA = "1111111111111111111111111111111111111111"
	prLink  = "https://github.com/acme/widgets/pull/42"
)

func fixtureTarget() run.Target {
	return run.Target{Schema: run.TargetSchema, Owner: "acme", Repo: "widgets", Number: 42, URL: prLink, Title: "Add widgets",
		Author: "author", Viewer: "reviewer", BaseSHA: "2222222222222222222222222222222222222222", HeadSHA: headSHA, Round: 1}
}

func finding(id, label string, blocking bool, loc *draft.Location) draft.Finding {
	return draft.Finding{ID: id, Rev: 1, Title: "Title " + id, Body: "Body of " + id, Location: loc, General: loc == nil,
		Label: label, Blocking: blocking, By: draft.ByAgent, Included: true, CreatedAt: fixtureNow, UpdatedAt: fixtureNow,
		History: []draft.HistoryEntry{}}
}

func accept(d *draft.Draft, id string) {
	d.Decisions[id] = draft.Decision{FindingID: id, Decision: draft.DecisionAccepted, FindingRev: 1, At: fixtureNow}
}

// readyDraft has accepted f-001 (blocking issue, a.go:3), f-002 (suggestion, a.go:10-12) and f-005 (general
// question), excluded f-003 and withdrawn f-004. Every text of the last two carries a marker word so a test can
// assert they appear nowhere.
func readyDraft() *draft.Draft {
	d := draft.NewEmpty()
	d.Version = 7
	d.Summary = "Looks mostly fine."
	excluded := finding("f-003", "issue", true, &draft.Location{Path: "a.go", Side: draft.SideRight, Line: 5})
	excluded.Title, excluded.Body, excluded.SuggestedFix = "EXCLUDED title", "EXCLUDED body", "EXCLUDED fix"
	withdrawn := finding("f-004", "issue", true, &draft.Location{Path: "a.go", Side: draft.SideRight, Line: 6})
	withdrawn.Title, withdrawn.Body, withdrawn.Included = "WITHDRAWN title", "WITHDRAWN body", false
	d.Findings = []draft.Finding{
		finding("f-001", "issue", true, &draft.Location{Path: "a.go", Side: draft.SideRight, Line: 3}),
		finding("f-002", "suggestion", false, &draft.Location{Path: "a.go", Side: draft.SideRight, Line: 12, StartLine: 10}),
		excluded,
		withdrawn,
		finding("f-005", "question", false, nil),
	}
	accept(d, "f-001")
	accept(d, "f-002")
	accept(d, "f-005")
	d.Decisions["f-003"] = draft.Decision{FindingID: "f-003", Decision: draft.DecisionExcluded, FindingRev: 1, At: fixtureNow}
	return d
}
