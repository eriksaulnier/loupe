package run

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
)

const sessionHelperEnv = "LOUPE_TEST_SESSION_HELPER_DIR"

// sessionHelper holds a review session until it is killed, so the test sees what the kernel does with a dead holder.
func sessionHelper(dir string) int {
	if _, err := HoldSession(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("held")
	_, _ = io.Copy(io.Discard, os.Stdin)
	return 0
}

func wantSessionOpen(t *testing.T, dir string, want bool) {
	t.Helper()
	open, err := SessionOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	if open != want {
		t.Fatalf("SessionOpen = %v, want %v", open, want)
	}
}

func TestSessionOpenFollowsHolders(t *testing.T) {
	dir := t.TempDir()
	wantSessionOpen(t, dir, false)

	first, err := HoldSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantSessionOpen(t, dir, true)
	second, err := HoldSession(dir)
	if err != nil {
		t.Fatalf("a second review session on the run: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	wantSessionOpen(t, dir, true)
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
	wantSessionOpen(t, dir, false)
}

func TestSessionOpenAfterHolderIsKilled(t *testing.T) {
	dir := t.TempDir()
	helper := exec.Command(os.Args[0])
	helper.Env = append(os.Environ(), sessionHelperEnv+"="+dir)
	helper.Stderr = os.Stderr
	stdin, err := helper.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := helper.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = helper.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "held\n" {
		t.Fatalf("helper did not hold the session: %q, %v", line, err)
	}
	wantSessionOpen(t, dir, true)

	if err := helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = helper.Wait()
	wantSessionOpen(t, dir, false)
}

func holdFor(t *testing.T, dir string) *Session {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := HoldSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Release() })
	return s
}

// The guard is per pull request: a new round is a new run, and a batch hands off several pull requests at once.
func TestOpenSessionCoversEveryRoundOfThePullRequest(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{RunDir(root, "o", "r", 1, 1), RunDir(root, "o", "r", 1, 2), RunDir(root, "o", "r", 1, 3), RunDir(root, "o", "r", 2, 1)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	want := func(ref Ref, wantRef string, wantOpen bool) {
		t.Helper()
		open, ok, err := OpenSession(root, ref)
		if err != nil {
			t.Fatal(err)
		}
		if ok != wantOpen || (ok && open.String() != wantRef) {
			t.Fatalf("OpenSession(%s) = %s, %v; want %s, %v", ref, open, ok, wantRef, wantOpen)
		}
	}
	round2 := Ref{Owner: "o", Repo: "r", Number: 1, Round: 2}
	other := Ref{Owner: "o", Repo: "r", Number: 2, Round: 1}
	want(round2, "", false)

	holdFor(t, RunDir(root, "o", "r", 1, 1))
	want(round2, "o/r#1@1", true)
	want(other, "", false)

	holdFor(t, RunDir(root, "o", "r", 1, 3))
	want(round2, "o/r#1@3", true)

	holdFor(t, RunDir(root, "o", "r", 1, 2))
	want(round2, "o/r#1@2", true)
}
