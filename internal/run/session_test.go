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
