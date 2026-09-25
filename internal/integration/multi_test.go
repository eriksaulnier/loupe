package integration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// publisher is one identity publishing on the shared pull request: a person on their own machine, or a pipeline whose
// every run starts from a fresh data root.
type publisher struct {
	login  string
	source string
	bot    bool
	home   string
	round  int
}

func human(t *testing.T, login string) *publisher {
	return &publisher{login: login, home: filepath.Join(t.TempDir(), "home")}
}

func pipeline(login, source string) *publisher {
	return &publisher{login: login, source: source, bot: true}
}

// publishAs files oneFinding as p and publishes it with --sticky, opening the review on message so each publisher's
// rounds can be told apart. It returns the --json result.
func (h *harness) publishAs(p *publisher, message string, flags ...string) map[string]any {
	h.t.Helper()
	h.GH.SetViewer(p.login)
	if p.bot {
		h.Home = filepath.Join(h.t.TempDir(), "home")
		h.UseInstallationToken()
		h.IsTerminal = true
		h.mustOK("capture", prURL(), "--source", p.source)
		h.mustOK("add", "--run", roundRef(1), "--from", h.WriteFile("finding.json", oneFinding))
		h.mustOK("summary", "--run", roundRef(1), "--body", message, "--expect-findings", "1")
		h.IsTerminal = false
		return h.mustOK(append([]string{"publish", roundRef(1), "--unattended"}, flags...)...)
	}
	h.Home = p.home
	h.GH.AllowUser()
	h.client, h.GitHubErr = h.GH.Client(h.t), nil
	p.round++
	h.captureRound(p.round)
	ref := roundRef(p.round)
	h.mustOK("add", "--run", ref, "--from", h.WriteFile("finding.json", oneFinding))
	h.IsTerminal = true
	h.Stdin = "a\nq\n"
	if _, stderr, exit := h.Run("review", ref, "--plain"); exit != 0 {
		h.t.Fatalf("review %s exit %d stderr %q", ref, exit, stderr)
	}
	h.Stdin = confirmPublish(message, "y")
	env, exit := h.RunJSON(append([]string{"publish", ref, "--plain"}, flags...)...)
	h.IsTerminal = false
	if exit != 0 || env["ok"] != true {
		h.t.Fatalf("publish %s as %s: exit %d envelope %v", ref, p.login, exit, env)
	}
	return env
}

// reviewsBy maps each author on the pull request to the bodies of their reviews, oldest first.
func (h *harness) reviewsBy() map[string][]string {
	h.t.Helper()
	reviews, err := h.GH.Client(h.t).ListReviews(context.Background(), owner, repo, number)
	if err != nil {
		h.t.Fatal(err)
	}
	by := map[string][]string{}
	for _, r := range reviews {
		by[r.User] = append(by[r.User], r.Body)
	}
	return by
}

// holdsOnly asserts body carries each of want and none of avoid.
func holdsOnly(t *testing.T, who, body string, want, avoid []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("%s's review lacks %q:\n%s", who, w, body)
		}
	}
	for _, a := range avoid {
		if strings.Contains(body, a) {
			t.Errorf("%s's review carries %q:\n%s", who, a, body)
		}
	}
}

// Two people each keep their own sticky review. Neither edits the other's, neither reads the other's rounds into their
// history, and each numbers and reads back only their own rounds, which live on their own machine.
func TestStickyTwoPeopleKeepTheirOwnReviews(t *testing.T) {
	h := newHarness(t)
	alice, bob := human(t, "alice"), human(t, "bob")
	for _, step := range []struct {
		p       *publisher
		message string
	}{{alice, "Alice one."}, {bob, "Bob one."}, {alice, "Alice two."}, {bob, "Bob two."}} {
		h.publishAs(step.p, step.message, "--sticky")
	}
	if h.GH.CreateCount() != 2 || h.GH.UpdateCount() != 2 {
		t.Fatalf("creates %d updates %d, want 2 and 2", h.GH.CreateCount(), h.GH.UpdateCount())
	}
	by := h.reviewsBy()
	if len(by["alice"]) != 1 || len(by["bob"]) != 1 {
		t.Fatalf("reviews by author %d and %d, want one each", len(by["alice"]), len(by["bob"]))
	}
	holdsOnly(t, "alice", by["alice"][0], []string{"Alice one.", "Alice two.", " round=2 ", " sticky=2 -->"}, []string{"Bob"})
	holdsOnly(t, "bob", by["bob"][0], []string{"Bob one.", "Bob two.", " round=2 ", " sticky=2 -->"}, []string{"Alice"})

	// --previous reads the local data root, so bob's previous round is his own.
	h.Home = bob.home
	previous := h.mustOK("show", "--previous", "--run", roundRef(2))
	if url, _ := previous["reviewUrl"].(string); !strings.Contains(url, "pullrequestreview-") || previous["round"] == nil {
		t.Fatalf("bob's previous round %v", previous)
	}
}

// A person and the pipeline each keep their own sticky review: a person's login never ends [bot], and the pipeline
// only edits a [bot] review.
func TestStickyPersonAndPipelineKeepTheirOwnReviews(t *testing.T) {
	h := newHarness(t)
	alice, ci := human(t, "alice"), pipeline("github-actions[bot]", "loupe-ci@1.0.0")
	for _, step := range []struct {
		p       *publisher
		message string
	}{{alice, "Alice one."}, {ci, "Pipeline one."}, {alice, "Alice two."}, {ci, "Pipeline two."}} {
		h.publishAs(step.p, step.message, "--sticky")
	}
	by := h.reviewsBy()
	if len(by["alice"]) != 1 || len(by["github-actions[bot]"]) != 1 {
		t.Fatalf("reviews by author: %d and %d, want one each", len(by["alice"]), len(by["github-actions[bot]"]))
	}
	holdsOnly(t, "alice", by["alice"][0], []string{"Alice one.", "Alice two.", " sticky=2 -->"}, []string{"Pipeline"})
	holdsOnly(t, "pipeline", by["github-actions[bot]"][0], []string{"Pipeline one.", "Pipeline two.", " sticky=2 -->"}, []string{"Alice"})
}

// Two Apps each keep their own sticky review, told apart by the source each capture records, since neither token can
// read its own login. Without that, one would pick the other's review, and GitHub would refuse that edit on every run.
// Unattended numbering still counts every App's loupe reviews, as specs/007-unattended-publish FR-015 defines it.
func TestStickyTwoAppsKeepTheirOwnReviews(t *testing.T) {
	h := newHarness(t)
	a, b := pipeline("loupe-ci[bot]", "loupe-ci@1.0.0"), pipeline("other-review[bot]", "other-review@3.1.0")
	for _, step := range []struct {
		p       *publisher
		message string
	}{{a, "A one."}, {b, "B one."}, {a, "A two."}, {b, "B two."}} {
		h.publishAs(step.p, step.message, "--sticky")
	}
	by := h.reviewsBy()
	if len(by["loupe-ci[bot]"]) != 1 || len(by["other-review[bot]"]) != 1 || h.GH.UpdateCount() != 2 {
		t.Fatalf("reviews by author %d and %d, updates %d; want one each and 2", len(by["loupe-ci[bot]"]), len(by["other-review[bot]"]), h.GH.UpdateCount())
	}
	holdsOnly(t, "App A", by["loupe-ci[bot]"][0], []string{"A one.", "A two.", " round=3 ", " sticky=2 -->"}, []string{"B one.", "B two."})
	holdsOnly(t, "App B", by["other-review[bot]"][0], []string{"B one.", "B two.", " round=4 ", " sticky=2 -->"}, []string{"A one.", "A two."})

	// A pipeline with no source could not tell its review from another's, so it is refused before anything is read.
	h.Home = filepath.Join(t.TempDir(), "home")
	h.IsTerminal = true
	h.mustOK("capture", prURL())
	h.mustOK("add", "--run", roundRef(1), "--from", h.WriteFile("finding.json", oneFinding))
	h.IsTerminal = false
	env, exit := h.RunJSON("publish", roundRef(1), "--unattended", "--sticky")
	refused, _ := env["error"].(map[string]any)
	if fix, _ := refused["fix"].(string); exit != 2 || refused["code"] != "usage" || !strings.Contains(fix, "--source") {
		t.Fatalf("exit %d envelope %v, want a usage refusal naming --source", exit, env)
	}
}

// A publisher's ordinary reviews are never edited: a sticky round only edits a review whose marker carries sticky=.
func TestStickyNeverEditsAnOrdinaryReview(t *testing.T) {
	h := newHarness(t)
	alice := human(t, "alice")
	h.publishAs(alice, "Plain review.", "--action", "comment")
	h.publishAs(alice, "Sticky one.", "--sticky")
	h.publishAs(alice, "Sticky two.", "--sticky")
	by := h.reviewsBy()
	if len(by["alice"]) != 2 || h.GH.CreateCount() != 2 || h.GH.UpdateCount() != 1 {
		t.Fatalf("reviews %d creates %d updates %d, want 2, 2 and 1", len(by["alice"]), h.GH.CreateCount(), h.GH.UpdateCount())
	}
	holdsOnly(t, "alice's ordinary", by["alice"][0], []string{"Plain review."}, []string{"sticky=", "Sticky"})
	holdsOnly(t, "alice's sticky", by["alice"][1], []string{"Sticky one.", "Sticky two.", " sticky=2 -->"}, []string{"Plain review."})
}
