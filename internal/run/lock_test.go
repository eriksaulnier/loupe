package run

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const lockHelperEnv = "LOUPE_TEST_LOCK_HELPER_DIR"

func TestMain(m *testing.M) {
	if dir := os.Getenv(lockHelperEnv); dir != "" {
		os.Exit(lockHelper(dir))
	}
	os.Exit(m.Run())
}

// lockHelper holds the lock until its stdin closes, then exits without unlocking so the kernel releases it.
func lockHelper(dir string) int {
	if _, err := Lock(dir, "helper hold", os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("locked")
	_, _ = io.Copy(io.Discard, os.Stdin)
	return 0
}

func timeoutEnv(ms string) func(string) string {
	return func(k string) string {
		if k == "LOUPE_LOCK_TIMEOUT_MS" {
			return ms
		}
		return ""
	}
}

func TestLockContention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".lock")

	helper := exec.Command(os.Args[0])
	helper.Env = append(os.Environ(), lockHelperEnv+"="+dir)
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
	if err != nil || line != "locked\n" {
		t.Fatalf("helper did not take the lock: %q, %v", line, err)
	}
	pid := strconv.Itoa(helper.Process.Pid)

	_, err = Lock(dir, "add", timeoutEnv("100"))
	r, ok := refusal.As(err)
	if !ok {
		t.Fatalf("expected refusal, got %v", err)
	}
	wantMessage := "run is locked: " + path + " is held by pid " + pid + " (helper hold)"
	wantFix := "wait for pid " + pid + " (helper hold) to finish, or stop it, then retry"
	if r.Code != refusal.Lock || r.Message != wantMessage || r.Fix != wantFix {
		t.Fatalf("got %+v", r)
	}

	acquired := make(chan error, 1)
	go func() {
		held, err := Lock(dir, "edit f-001", timeoutEnv("5000"))
		if err == nil {
			err = held.Unlock()
		}
		acquired <- err
	}()
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := helper.Wait(); err != nil {
		t.Fatalf("helper: %v", err)
	}
	if err := <-acquired; err != nil {
		t.Fatalf("waiter did not acquire the lock: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lock file must survive unlock: %v", err)
	}
	if want := strconv.Itoa(os.Getpid()) + " edit f-001"; string(content) != want {
		t.Fatalf("lock file content %q, want %q", content, want)
	}
}

func TestLockTimeoutValidation(t *testing.T) {
	for _, v := range []string{"abc", "-1", "60001", "1.5"} {
		_, err := Lock(t.TempDir(), "add", timeoutEnv(v))
		r, ok := refusal.As(err)
		if !ok || r.Code != refusal.Usage {
			t.Fatalf("%q: expected usage refusal, got %v", v, err)
		}
	}
	for _, v := range []string{"", "0", "60000"} {
		held, err := Lock(t.TempDir(), "add", timeoutEnv(v))
		if err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		if err := held.Unlock(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHeldRefusalWithoutHolder(t *testing.T) {
	r, ok := refusal.As(heldRefusal(filepath.Join(t.TempDir(), ".lock")))
	if !ok || r.Code != refusal.Lock || strings.Contains(r.Fix, "rm ") {
		t.Fatalf("got %+v", r)
	}
}
