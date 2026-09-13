package gitrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewBuildsPullRequest(t *testing.T) {
	r := New(t, "acme", "widgets", 42)
	if r.BaseSHA() == r.HeadSHA() || len(r.HeadSHA()) != 40 {
		t.Fatalf("base %s head %s", r.BaseSHA(), r.HeadSHA())
	}
	if got := r.git(t, r.Dir, "config", "remote.origin.url"); got != "https://github.com/acme/widgets.git" {
		t.Fatalf("origin url %q", got)
	}
	if got, err := r.run(r.Dir, "config", "core.hooksPath"); err == nil {
		t.Fatalf("global config leaked: core.hooksPath=%q", got)
	}
	if got := r.git(t, r.Dir, "rev-parse", "HEAD"); got != r.BaseSHA() {
		t.Fatalf("clone HEAD %s, want base %s", got, r.BaseSHA())
	}
	if _, err := r.run(r.Dir, "cat-file", "-e", r.HeadSHA()); err == nil {
		t.Fatal("the clone must not have the head commit before fetching")
	}

	r.git(t, r.Dir, "fetch", "origin", "refs/pull/42/head:refs/loupe/acme/widgets/42/1/head")
	if got := r.git(t, r.Dir, "rev-parse", "refs/loupe/acme/widgets/42/1/head"); got != r.HeadSHA() {
		t.Fatalf("fetched head %s, want %s", got, r.HeadSHA())
	}

	status := r.git(t, r.Dir, "diff", "--name-status", "--find-renames", r.BaseSHA(), r.HeadSHA())
	for _, want := range []string{"M\tsrc/app.go", "A\tsrc/added.go", "D\tremove-me.txt", "R100\tdocs/old-name.md\tdocs/new-name.md"} {
		if !strings.Contains(status, want) {
			t.Fatalf("diff missing %q:\n%s", want, status)
		}
	}
	hunks := strings.Count(r.git(t, r.Dir, "diff", r.BaseSHA(), r.HeadSHA(), "--", "src/app.go"), "\n@@ ")
	if hunks < 2 {
		t.Fatalf("src/app.go has %d hunks, want at least 2", hunks)
	}
}

func TestPushHeadMovesPullRequest(t *testing.T) {
	r := New(t, "acme", "widgets", 7)
	old := r.HeadSHA()
	sha := r.PushHead(map[string]string{"src/added.go": "package src\n\nconst Moved = true\n"})
	if sha == old || sha != r.HeadSHA() {
		t.Fatalf("PushHead returned %s, old %s, HeadSHA %s", sha, old, r.HeadSHA())
	}
	r.git(t, r.Dir, "fetch", "origin", r.HeadSHA())
	if got := r.git(t, r.Dir, "rev-parse", "FETCH_HEAD"); got != sha {
		t.Fatalf("fetch by sha got %s, want %s", got, sha)
	}
	if got := r.git(t, r.Dir, "rev-parse", sha+"^"); got != old {
		t.Fatalf("new head parent %s, want previous head %s", got, old)
	}
}

func TestSnapshotDiff(t *testing.T) {
	r := New(t, "acme", "widgets", 1)
	before := r.Snapshot()
	r.git(t, r.Dir, "fetch", "origin", "refs/pull/1/head:refs/loupe/acme/widgets/1/1/head")
	if diffs := before.DiffIgnoringLoupeRefs(r.Snapshot()); len(diffs) != 0 {
		t.Fatalf("loupe refs must be ignored: %v", diffs)
	}

	r.git(t, r.Dir, "update-ref", "refs/heads/other", "HEAD")
	if err := os.WriteFile(filepath.Join(r.Dir, "src", "app.go"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diffs := before.DiffIgnoringLoupeRefs(r.Snapshot())
	joined := strings.Join(diffs, "\n")
	if !strings.Contains(joined, "refs/heads/other") || !strings.Contains(joined, "status") {
		t.Fatalf("expected ref and status differences, got %v", diffs)
	}

	r.git(t, r.Dir, "add", "src/app.go")
	if diffs := before.DiffIgnoringLoupeRefs(r.Snapshot()); !strings.Contains(strings.Join(diffs, "\n"), "index") {
		t.Fatalf("expected an index difference, got %v", diffs)
	}
}
