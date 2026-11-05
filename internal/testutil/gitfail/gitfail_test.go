package gitfail

import (
	"os/exec"
	"strings"
	"testing"
)

func git(t *testing.T, args ...string) (string, int) {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	if exit, ok := err.(*exec.ExitError); ok {
		return string(out), exit.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(out), 0
}

func TestWhenContainsFailsOnlyAMatchingCall(t *testing.T) {
	WhenContains(t, "version --build")
	if out, code := git(t, "version", "--build-options"); code != 2 || out != "injected failure\n" {
		t.Fatalf("matching call: exit %d, %q", code, out)
	}
	if out, code := git(t, "version"); code != 0 || !strings.HasPrefix(out, "git version ") {
		t.Fatalf("other call: exit %d, %q", code, out)
	}
}

func TestWhenArgFailsOnlyOnAWholeArgument(t *testing.T) {
	WhenArg(t, "--build-options", 3)
	if out, code := git(t, "version", "--build-options"); code != 3 || out != "injected failure\n" {
		t.Fatalf("matching call: exit %d, %q", code, out)
	}
	if _, code := git(t, "version", "--build-option"); code == 3 {
		t.Fatal("a prefix of the argument failed")
	}
	if _, code := git(t, "version", ""); code != 0 {
		t.Fatalf("an empty argument: exit %d", code)
	}
}

func TestSilentWhenArgPrintsNothing(t *testing.T) {
	SilentWhenArg(t, "version")
	if out, code := git(t, "version"); code != 0 || out != "" {
		t.Fatalf("matching call: exit %d, %q", code, out)
	}
	if out, code := git(t, "--version"); code != 0 || !strings.HasPrefix(out, "git version ") {
		t.Fatalf("other call: exit %d, %q", code, out)
	}
}
