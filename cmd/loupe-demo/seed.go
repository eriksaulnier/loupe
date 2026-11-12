package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/local"
	"github.com/eriksaulnier/loupe/internal/mode"
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
		Location:   &draft.LocationInput{Path: "internal/cache/store.go", Line: 38},
		Label:      "issue",
		Confidence: "medium",
	},
	{
		Title: "Cache entries never expire when the ETag is missing",
		Body: "When the upstream response carries no ETag, `Get` returns the entry as fresh forever. A pull request whose metadata changes after the first fetch keeps its stale title and head SHA until the process restarts.\n\n" +
			"The TTL comparison on line 93 is never reached for those entries, so the cache goes stale exactly for the repositories that don't send ETags.",
		Location:     &draft.LocationInput{Path: "internal/cache/store.go", StartLine: 90, Line: 91},
		Label:        "issue",
		Blocking:     true,
		Confidence:   "high",
		Severity:     "major",
		Verified:     "reproduced",
		Impact:       "Any repository whose API omits ETags shows the title and head SHA from the first fetch until restart, so a force-push is reviewed against the wrong commit.",
		References:   []string{"https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api#use-conditional-requests-if-appropriate"},
		SuggestedFix: "Fall through to the TTL comparison when ETag is empty instead of returning early.",
	},
	{
		Title:        "Hit ratio counters reset on every scrape",
		Body:         "`Collect` stores zero into both counters after reading them. A second scraper, or a retried scrape, computes the ratio from a fraction of the traffic.",
		Location:     &draft.LocationInput{Path: "internal/cache/metrics.go", StartLine: 39, Line: 40},
		Label:        "suggestion",
		Confidence:   "medium",
		Severity:     "minor",
		Verified:     "plausible",
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
		Location: &draft.LocationInput{Path: "internal/cache/store.go", Line: 42, Side: draft.SideLeft},
		Label:    "issue",
	},
	{
		Title:    "Flag default duplicates the config file value",
		Body:     "The cache size default is set here and in the config file.",
		Location: &draft.LocationInput{Path: "cmd/widgets/main.go", Line: 11},
		Label:    "issue",
	},
}

// demoPRState is the live state LOUPE_DEMO_PR_STATE asks every demo pull request to report, so the review header's
// tag and the confirmation's state can be seen by hand (specs/050-pr-state-awareness).
func demoPRState(value string) (state string, merged bool, err error) {
	switch value {
	case "":
		return "open", false, nil
	case "merged":
		return "closed", true, nil
	case "closed":
		return "closed", false, nil
	}
	return "", false, fmt.Errorf("LOUPE_DEMO_PR_STATE is %q; use merged or closed, or leave it unset", value)
}

// seed tells the fake GitHub about the three demo pull requests and, when writeRuns is set, writes their runs and the
// local run.
func seed(home string, gh *fakegh.Server, now time.Time, writeRuns bool, prState string) error {
	state, merged, err := demoPRState(prState)
	if err != nil {
		return err
	}
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
		target, d, err := demoRun(parsed, r.number, r.decide, now)
		if err != nil {
			return err
		}
		data, err := json.Marshal(d)
		if err != nil {
			return err
		}
		url := target.URL
		if writeRuns {
			if err := run.CreateRun(run.RunDir(home, owner, repo, r.number, 1), target, []byte(demoDiff), data, nil); err != nil {
				return fmt.Errorf("create demo run #%d: %w", r.number, err)
			}
		}
		gh.SetPR(owner, repo, github.PullRequest{
			Number: r.number, URL: url, Title: prTitle, State: state, Merged: merged, Author: author, BaseRef: "main", BaseSHA: baseSHA, HeadSHA: r.live,
		})
	}
	if writeRuns {
		if err := seedLocal(home, now); err != nil {
			return err
		}
	}
	gained := []github.Commit{
		{SHA: "3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d", Message: "fix(cache): fall through to the TTL check without an ETag"},
		{SHA: "4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e", Message: "test(cache): cover the 304 path"},
		{SHA: movedSHA, Message: "docs: note the cache size limit"},
	}
	gh.SetCommits(owner, repo, 44, append([]github.Commit{{SHA: headSHA, Message: "feat(cache): serve 304s from the store"}}, gained...)...)
	qualifier := owner + ":" + repo + ":"
	gh.SetComparison(owner, repo, qualifier+headSHA, qualifier+movedSHA, github.Comparison{
		Status:  "ahead",
		AheadBy: 3,
		Commits: gained,
		Files: []github.ComparedFile{{Filename: "internal/cache/store.go", Additions: 6, Deletions: 2},
			{Filename: "internal/cache/store_test.go", Additions: 24}, {Filename: "README.md", Additions: 2}},
	})
	return nil
}

// demoRun is one seeded run's target and draft, decided by decide.
func demoRun(parsed *diff.Diff, number int, decide func(d *draft.Draft) error, now time.Time) (run.Target, *draft.Draft, error) {
	d := draft.NewEmpty()
	if _, err := draft.Add(d, findings, parsed, draft.Rules{}, draft.ByAgent, now); err != nil {
		return run.Target{}, nil, fmt.Errorf("add the demo findings: %w", err)
	}
	d.Summary = summary
	if err := decide(d); err != nil {
		return run.Target{}, nil, fmt.Errorf("decide the demo findings for #%d: %w", number, err)
	}
	return demoTarget(number, headSHA, 1, now), d, nil
}

func demoTarget(number int, head string, round int, now time.Time) run.Target {
	url := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number)
	return run.Target{
		Schema: run.TargetSchema, Owner: owner, Repo: repo, Number: number, URL: url, Title: prTitle,
		Author: author, Viewer: viewer, BaseSHA: baseSHA, HeadSHA: head, Round: round, CapturedAt: now,
		DiffSHA256: run.DiffSHA256([]byte(demoDiff)), Source: "code-review@1.4.0", Model: "anthropic/claude-opus-5.5",
	}
}

// midReview leaves something in every state: accepted, pending and blocking, pending with an answered note, excluded
// and withdrawn.
func midReview(parsed *diff.Diff, now time.Time) func(d *draft.Draft) error {
	return func(d *draft.Draft) error {
		for _, id := range []string{"f-001", "f-006"} {
			if _, err := draft.Accept(d, id, "", now); err != nil {
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
		reason := draft.EditInput{WithdrawReason: json.RawMessage(`{"text": "Both values agree, so nothing runs differently.", "code": "no-runtime-effect"}`)}
		_, _, err = draft.Edit(d, "f-007", reason, &withdrawn, parsed, draft.ByAgent, draft.Rules{}, now)
		return err
	}
}

// readyToPublish accepts the findings a reviewer would keep and excludes the rest.
func readyToPublish(now time.Time) func(d *draft.Draft) error {
	return func(d *draft.Draft) error {
		for _, f := range d.Findings {
			decide := func(d *draft.Draft, id string, now time.Time) ([]string, error) {
				return draft.Accept(d, id, "", now)
			}
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

// localRef is the seeded local run, which review opens in apply mode.
const localRef = owner + "/" + repo + ":local#1"

// localFiles are the plan and the code file the local run captures with --files, written under the data root so
// review can draw them.
var localFiles = map[string]string{
	"docs/cache-plan.md": `# Cache plan

## Goals

Serve pull request metadata from memory, so a review opens without a round trip to GitHub.

## Storage

Keep entries in a map guarded by one lock.
Evict the oldest entry once the map holds 5,000 entries.

## Expiry

An entry with an ETag is revalidated with a conditional request after one minute.
An entry without an ETag never expires.

## Rollout

Ship behind the cache-size flag, off by default for one release.
`,
	"internal/cache/store.go": `package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Store is a read-through cache in front of the metadata client.
type Store struct {
	mu      sync.RWMutex
	client  Client
	entries map[string]Entry
	order   []string
	ttl     time.Duration
	now     func() time.Time
}

// Put fetches the metadata for key and stores it.
func (s *Store) Put(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, err := s.client.Metadata(ctx, key)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", key, err)
	}
	s.entries[key] = Entry{Meta: meta, ETag: meta.ETag, At: s.now()}
	s.order = append(s.order, key)
	return nil
}

// Get returns the cached entry and whether it is still fresh.
func (s *Store) Get(key string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key]
	if !ok || e.ETag == "" {
		return e, ok
	}
	return e, s.now().Sub(e.At) < s.ttl
}
`,
}

var localFindings = []draft.FindingInput{
	{
		Title:    "Entries without an ETag never expire",
		Body:     "The plan keeps an entry with no ETag forever, so a repository that sends none shows a stale title and head until the process restarts.",
		Location: &draft.LocationInput{Path: "docs/cache-plan.md", Heading: ptr("Expiry")},
		Severity: "major",
		Proposal: "Give every entry the one-minute TTL, and revalidate only the entries that carry an ETag.",
	},
	{
		Title:    "Put holds the write lock across the network call",
		Body:     "`Put` takes the write lock before it calls GitHub, so every `Get` waits behind that round trip.",
		Location: &draft.LocationInput{Path: "internal/cache/store.go", StartLine: 22, Line: 27},
		Severity: "major",
		Proposal: "Fetch before taking the lock:\n\n```go\nmeta, err := s.client.Metadata(ctx, key)\nif err != nil {\n\treturn fmt.Errorf(\"fetch %s: %w\", key, err)\n}\ns.mu.Lock()\ndefer s.mu.Unlock()\n```",
	},
	{
		Title:    "Evict by count or by memory?",
		Body:     "The plan evicts at 5,000 entries. Counting bytes instead bounds memory when one pull request carries very large metadata, at the cost of sizing each entry.",
		Location: &draft.LocationInput{Path: "docs/cache-plan.md", Line: 10},
		Severity: "minor",
		Proposal: "Keep the count limit, and note in the plan that a byte limit is the next step if memory grows.",
	},
	{
		Title:   "Who invalidates an entry on a force-push?",
		Body:    "Nothing in the plan clears an entry when the head moves. The choices are a webhook that deletes the entry, a shorter TTL, or comparing the head SHA on every read.",
		General: true,
	},
}

func ptr(s string) *string { return &s }

// seedLocal writes the plan and the code file under home, captures them as loupe capture --files does, and files the
// local findings and a summary, all pending.
func seedLocal(home string, now time.Time) error {
	src := filepath.Join(home, "src", "widgets")
	paths := []string{"docs/cache-plan.md", "internal/cache/store.go"}
	for _, path := range paths {
		abs := filepath.Join(src, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(localFiles[path]), 0o644); err != nil {
			return err
		}
	}
	captured, err := local.Capture(local.Request{Form: local.FormFiles, WorkDir: src, Files: paths})
	if err != nil {
		return fmt.Errorf("capture the demo local run: %w", err)
	}
	// The demo source sits in a temporary directory with no origin or branch. It stands for the acme/widgets checkout
	// on the pull request's branch.
	captured.Local.Prefix, captured.Local.Repository, captured.Local.Branch = owner+"/"+repo, owner+"/"+repo, "feat/cache"
	target := run.Target{
		Kind: run.KindLocal, Title: "Cache plan", Applier: "Claude", CapturedAt: now.UTC(),
		DiffSHA256: run.DiffSHA256(captured.Diff), Local: &captured.Local,
	}
	parsed, err := diff.Parse(captured.Diff)
	if err != nil {
		return fmt.Errorf("parse the demo local diff: %w", err)
	}
	parsed.Outline = diff.Outline{}
	for _, e := range captured.Outline {
		parsed.Outline[e.Path] = e.Headings
	}
	d := draft.NewEmpty()
	if _, err := draft.Add(d, localFindings, parsed, mode.Of(target).Rules(), draft.ByAgent, now); err != nil {
		return fmt.Errorf("add the demo local findings: %w", err)
	}
	d.Summary = "Four decisions on the cache plan before I change the store. The force-push question has no proposal: pick one of the choices or name another."
	draftJSON, err := json.Marshal(d)
	if err != nil {
		return err
	}
	outlineJSON, err := run.EncodeOutline(captured.Outline)
	if err != nil {
		return err
	}
	if _, _, err := run.NextLocal(home, "loupe-demo", func(string) string { return "" }, target, captured.Diff, draftJSON,
		map[string][]byte{run.OutlineFile: outlineJSON}); err != nil {
		return fmt.Errorf("create the demo local run: %w", err)
	}
	return nil
}
