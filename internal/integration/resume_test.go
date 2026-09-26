package integration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/github"
)

const oneFinding = `{"title": "Changed line", "body": "Evidence.", "location": {"path": "src/app.go", "line": 3}, "label": "issue"}`

func pullURL(n int) string { return fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, n) }

func listRuns(t *testing.T, env map[string]any) []map[string]any {
	t.Helper()
	raw, _ := env["runs"].([]any)
	runs := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]any)
		runs = append(runs, m)
	}
	return runs
}

func TestResumeFromBranch(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, pr := range []struct {
		number int
		branch string
	}{{7, "feature-a"}, {8, "feature-b"}} {
		h.Repo.AddPull(pr.number)
		h.GH.SetPR(owner, repo, github.PullRequest{
			Number: pr.number, URL: pullURL(pr.number), Title: "Work on " + pr.branch, State: "open", Author: "author",
			BaseRef: "main", BaseSHA: h.Repo.BaseSHA(), HeadSHA: h.Repo.HeadSHA(),
		})
		h.GH.SetBranch(owner, repo, pr.number, pr.branch)
	}

	h.Repo.Git("checkout", "--quiet", "-b", "feature-a")
	earlier := func() time.Time { return fixedNow().Add(-time.Hour) }
	if _, stderr, exit := h.runWith("", earlier, "capture", pullURL(7)); exit != 0 {
		t.Fatalf("capture 7 exit %d stderr %q", exit, stderr)
	}
	h.Repo.Git("checkout", "--quiet", "-b", "feature-b")
	h.mustOK("capture", pullURL(8))

	h.mustOK("add", "--from", h.WriteFile("finding.json", oneFinding))
	env := h.mustOK("show")
	if env["run"] != "acme/widgets#8@1" {
		t.Fatalf("show resolved %v", env["run"])
	}
	h.IsTerminal = true
	h.Stdin = "a\nq\n"
	if _, stderr, exit := h.Run("review", "--plain"); exit != 0 {
		t.Fatalf("review exit %d stderr %q", exit, stderr)
	}
	if d, _ := h.mustOK("show", "--run", "acme/widgets#8")["dispositions"].(map[string]any); d["f-001"] != "accepted" {
		t.Fatalf("review from the branch did not decide #8: %v", d)
	}
	h.mustOK("add", "--run", "acme/widgets#7", "--from", h.WriteFile("finding.json", oneFinding))

	checkList := func(wantStates ...string) {
		t.Helper()
		runs := listRuns(t, h.mustOK("list"))
		if len(runs) != 2 {
			t.Fatalf("list runs %v", runs)
		}
		for i, want := range []struct {
			ref, url, title, capturedAt string
			counts                      map[string]string
		}{
			{"acme/widgets#8@1", pullURL(8), "Work on feature-b", "2026-09-13T12:00:00Z", map[string]string{"accepted": "1", "pending": "0"}},
			{"acme/widgets#7@1", pullURL(7), "Work on feature-a", "2026-09-13T11:00:00Z", map[string]string{"accepted": "0", "pending": "1"}},
		} {
			got := runs[i]
			if got["ref"] != want.ref || got["url"] != want.url || got["title"] != want.title || got["capturedAt"] != want.capturedAt ||
				fmt.Sprint(got["round"]) != "1" || got["state"] != wantStates[i] {
				t.Errorf("run %d: %v", i, got)
			}
			counts, _ := got["counts"].(map[string]any)
			for _, key := range []string{"accepted", "pending", "excluded", "withdrawn", "openNotes"} {
				wantCount := want.counts[key]
				if wantCount == "" {
					wantCount = "0"
				}
				if fmt.Sprint(counts[key]) != wantCount {
					t.Errorf("run %d counts %v", i, counts)
				}
			}
		}
	}
	checkList("ready", "captured")

	h.Stdin = confirmPublish("", "y")
	if stdout, stderr, exit := h.Run("publish", "--action", "comment", "--plain"); exit != 0 {
		t.Fatalf("publish exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(h.Home, "runs", owner, repo, "8", "1", "receipt.json")); err != nil {
		t.Fatalf("publish from the branch did not publish #8: %v", err)
	}
	checkList("published", "captured")

	stdout, stderr, exit := h.Run("list")
	if exit != 0 || !strings.Contains(stdout, "acme/widgets#8@1") || !strings.Contains(stdout, "published") || !strings.Contains(stdout, "acme/widgets#7@1") {
		t.Fatalf("list exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) < 2 || strings.Index(lines[len(lines)-2], "published") != strings.Index(lines[len(lines)-1], "captured") {
		t.Fatalf("list rows are not aligned:\n%s", stdout)
	}

	h.Repo.Git("checkout", "--quiet", "-b", "lonely")
	errObj := h.mustRefuse("no-run", "review")
	if fix, _ := errObj["fix"].(string); strings.Contains(fix, "--run") || !strings.Contains(fix, "loupe review <ref>") {
		t.Fatalf("review no-run fix %q", fix)
	}
	errObj = h.mustRefuse("no-run", "publish", "acme/widgets#99", "--action", "comment")
	if fix, _ := errObj["fix"].(string); strings.Contains(fix, "--run") || !strings.Contains(fix, "loupe publish <ref>") {
		t.Fatalf("publish no-run fix %q", fix)
	}
	if fix, _ := h.mustRefuse("no-run", "show")["fix"].(string); !strings.Contains(fix, "--run <ref>") {
		t.Fatalf("show no-run fix %q", fix)
	}

	h.GitHubErr = errors.New("no GitHub credentials")
	h.mustOK("show", "--run", "acme/widgets#7")
	h.Env["LOUPE_RUN"] = "acme/widgets#7"
	h.mustOK("show")
	delete(h.Env, "LOUPE_RUN")
	h.WorkDir = t.TempDir()
	h.mustOK("list")
	h.mustRefuse("no-run", "show")

	draftPath := filepath.Join(h.Home, "runs", owner, repo, "7", "1", "draft.json")
	if err := os.WriteFile(draftPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	errObj = h.mustRefuse("record", "list")
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, draftPath) {
		t.Fatalf("record refusal does not name %s: %v", draftPath, errObj)
	}
}
