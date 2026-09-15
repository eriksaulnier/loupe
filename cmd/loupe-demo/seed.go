package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

const (
	owner    = "acme"
	repo     = "widgets"
	author   = "alex-dev"
	viewer   = "sam-reviewer"
	baseSHA  = "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a"
	headSHA  = "1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b"
	movedSHA = "2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c"
	prTitle  = "feat(cache): add a read-through cache for pull request metadata"
	summary  = "Adds a read-through cache in front of the GitHub metadata client. One blocking concern around expiry in " +
		"store.go; the locking, metrics and docs findings are minor. Tests pass locally, but the 304 path is not " +
		"exercised by the fake server."
)

// demoDiff is a small, real-looking change: a cache store, its metrics, and a flag default.
const demoDiff = `diff --git a/internal/cache/store.go b/internal/cache/store.go
index 1111111..2222222 100644
--- a/internal/cache/store.go
+++ b/internal/cache/store.go
@@ -34,11 +34,12 @@ type Store struct
 // Put fetches the metadata for key and stores it.
 func (s *Store) Put(ctx context.Context, key string) error {
 	s.mu.Lock()
 	defer s.mu.Unlock()
-	meta, err := s.client.Metadata(ctx, key)
+	meta, err := s.fetch(ctx, key)
 	if err != nil {
 		return fmt.Errorf("fetch %s: %w", key, err)
 	}
-	s.entries[key] = Entry{Meta: meta}
+	s.entries[key] = Entry{Meta: meta, ETag: meta.ETag, At: s.now()}
+	s.order = append(s.order, key)
 	return nil
 }
@@ -80,7 +81,14 @@ func (s *Store) Put

 // Get returns the cached entry and whether it is still fresh.
 func (s *Store) Get(key string) (Entry, bool) {
 	s.mu.RLock()
 	defer s.mu.RUnlock()
-	return s.entries[key], true
+	e, ok := s.entries[key]
+	if !ok {
+		return Entry{}, false
+	}
+	if e.ETag == "" {
+		return e, true
+	}
+	return e, s.now().Sub(e.At) < s.ttl
 }
diff --git a/internal/cache/metrics.go b/internal/cache/metrics.go
index 3333333..4444444 100644
--- a/internal/cache/metrics.go
+++ b/internal/cache/metrics.go
@@ -36,6 +36,8 @@ func NewMetrics() *Metrics
 // Collect reports the hit ratio since the last scrape.
 func (m *Metrics) Collect(ch chan<- prometheus.Metric) {
 	hits, misses := m.hits.Load(), m.misses.Load()
+	m.hits.Store(0)
+	m.misses.Store(0)
 	ch <- prometheus.MustNewConstMetric(m.ratio, prometheus.GaugeValue, ratio(hits, misses))
 }

diff --git a/cmd/widgets/main.go b/cmd/widgets/main.go
index 5555555..6666666 100644
--- a/cmd/widgets/main.go
+++ b/cmd/widgets/main.go
@@ -10,5 +10,5 @@ func main() {
 	flags := flag.NewFlagSet("widgets", flag.ExitOnError)
-	size := flags.Int("cache-size", 1000, "entries to keep")
+	size := flags.Int("cache-size", 5000, "entries to keep")
 	ttl := flags.Duration("cache-ttl", time.Minute, "how long an entry stays fresh")
 	flags.Parse(os.Args[1:])
 	run(*size, *ttl)
`

var findings = []draft.FindingInput{
	{
		Title:      "Store.Put holds the write lock across the network call",
		Body:       "`Put` takes the write lock and then calls GitHub. Every `Get` waits behind that round trip for as long as it takes, so one slow response stalls every reader.",
		Location:   &draft.Location{Path: "internal/cache/store.go", Line: 38},
		Label:      "issue",
		Confidence: "medium",
	},
	{
		Title: "Cache entries never expire when the ETag is missing",
		Body: "When the upstream response carries no ETag, `Get` returns the entry as fresh forever. A pull request whose metadata changes after the first fetch keeps its stale title and head SHA until the process restarts.\n\n" +
			"The TTL comparison on line 93 is never reached for those entries, so the cache goes stale exactly for the repositories that don't send ETags.",
		Location:     &draft.Location{Path: "internal/cache/store.go", StartLine: 90, Line: 91},
		Label:        "issue",
		Blocking:     true,
		Confidence:   "high",
		Severity:     "major",
		SuggestedFix: "Fall through to the TTL comparison when ETag is empty instead of returning early.",
	},
	{
		Title:        "Hit ratio counters reset on every scrape",
		Body:         "`Collect` stores zero into both counters after reading them. A second scraper, or a retried scrape, computes the ratio from a fraction of the traffic.",
		Location:     &draft.Location{Path: "internal/cache/metrics.go", StartLine: 39, Line: 40},
		Label:        "suggestion",
		Confidence:   "medium",
		SuggestedFix: "Export hits and misses as counters and let the dashboard compute the ratio.",
	},
	{
		Title:   "Is the 304 path covered by the fake server?",
		Body:    "The client sends `If-None-Match`, but the fake server never answers 304, so the path that reuses a cached entry is untested.",
		General: true,
		Label:   "question",
	},
	{
		Title:   "Document the cache size limit in the README",
		Body:    "The default of 5,000 entries appears only in the flag help. Someone tuning memory will look in the README first.",
		General: true,
		Label:   "suggestion",
	},
	{
		Title:    "The old store dropped insertion order",
		Body:     "Before this change nothing recorded the order entries arrived in, so eviction could not be oldest-first.",
		Location: &draft.Location{Path: "internal/cache/store.go", Line: 42, Side: draft.SideLeft},
		Label:    "issue",
	},
	{
		Title:    "Flag default duplicates the config file value",
		Body:     "The cache size default is set here and in the config file.",
		Location: &draft.Location{Path: "cmd/widgets/main.go", Line: 11},
		Label:    "issue",
	},
}

// seed writes the three demo runs and tells the fake GitHub about their pull requests.
func seed(home string, gh *fakegh.Server, now time.Time) error {
	parsed, err := diff.Parse([]byte(demoDiff))
	if err != nil {
		return fmt.Errorf("parse the demo diff: %w", err)
	}
	gh.SetViewer(viewer)
	runs := []struct {
		number int
		decide func(d *draft.Draft) error
		live   string
	}{
		{42, midReview(parsed, now), headSHA},
		{43, readyToPublish(now), headSHA},
		{44, readyToPublish(now), movedSHA},
	}
	for _, r := range runs {
		d := draft.NewEmpty()
		if _, err := draft.Add(d, findings, parsed, draft.ByAgent, now); err != nil {
			return fmt.Errorf("add the demo findings: %w", err)
		}
		d.Summary = summary
		if err := r.decide(d); err != nil {
			return fmt.Errorf("decide the demo findings for #%d: %w", r.number, err)
		}
		data, err := json.Marshal(d)
		if err != nil {
			return err
		}
		url := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, r.number)
		target := run.Target{
			Schema: run.TargetSchema, Owner: owner, Repo: repo, Number: r.number, URL: url, Title: prTitle,
			Author: author, Viewer: viewer, BaseSHA: baseSHA, HeadSHA: headSHA, Round: 1, CapturedAt: now,
			DiffSHA256: run.DiffSHA256([]byte(demoDiff)), Source: "loupe-demo",
		}
		if err := run.CreateRun(run.RunDir(home, owner, repo, r.number, 1), target, []byte(demoDiff), data); err != nil {
			return fmt.Errorf("create demo run #%d: %w", r.number, err)
		}
		gh.SetPR(owner, repo, github.PullRequest{
			Number: r.number, URL: url, Title: prTitle, State: "open", Author: author, BaseRef: "main", BaseSHA: baseSHA, HeadSHA: r.live,
		})
	}
	qualifier := owner + ":" + repo + ":"
	gh.SetComparison(owner, repo, qualifier+headSHA, qualifier+movedSHA, github.Comparison{
		Status:  "ahead",
		AheadBy: 3,
		Commits: []github.Commit{
			{SHA: "3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d", Message: "fix(cache): fall through to the TTL check without an ETag"},
			{SHA: "4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e", Message: "test(cache): cover the 304 path"},
			{SHA: movedSHA, Message: "docs: note the cache size limit"},
		},
		Files: []github.ComparedFile{{Filename: "internal/cache/store.go"}, {Filename: "README.md"}},
	})
	return nil
}

// midReview leaves something in every state: accepted, pending and blocking, pending with an answered note, excluded
// and withdrawn.
func midReview(parsed *diff.Diff, now time.Time) func(d *draft.Draft) error {
	return func(d *draft.Draft) error {
		for _, id := range []string{"f-001", "f-006"} {
			if _, err := draft.Accept(d, id, now); err != nil {
				return err
			}
		}
		if _, err := draft.Exclude(d, "f-005", now); err != nil {
			return err
		}
		note, err := draft.SendBack(d, "f-003", "Is the reset intentional? We only run one Prometheus.", now)
		if err != nil {
			return err
		}
		if _, err := draft.AddReply(d, note.ID, "Kept as a suggestion: a retried scrape also halves the ratio, not only a second scraper.", draft.ByAgent, now); err != nil {
			return err
		}
		withdrawn := false
		_, _, err = draft.Edit(d, "f-007", draft.EditInput{}, &withdrawn, parsed, draft.ByAgent, now)
		return err
	}
}

// readyToPublish accepts the findings a reviewer would keep and excludes the rest.
func readyToPublish(now time.Time) func(d *draft.Draft) error {
	return func(d *draft.Draft) error {
		for _, f := range d.Findings {
			decide := draft.Accept
			if f.ID == "f-005" || f.ID == "f-007" {
				decide = draft.Exclude
			}
			if _, err := decide(d, f.ID, now); err != nil {
				return err
			}
		}
		return nil
	}
}
