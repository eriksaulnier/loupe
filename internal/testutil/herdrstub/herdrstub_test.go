package herdrstub

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func stub(t *testing.T, script string) (bin, log string) {
	t.Helper()
	bin = t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(bin, "herdr"), filepath.Join(bin, "argv")
}

func TestScriptAnswersEachStepAndLogsArgv(t *testing.T) {
	bin, log := stub(t, Script(`{"height":9,"width":7}`, "", ""))
	out, err := exec.Command(bin, "pane", "layout", "--pane", "w1:p1").Output()
	if err != nil || !strings.Contains(string(out), `"pane_id":"w1:p1","rect":{"height":9,"width":7}`) {
		t.Fatalf("layout: %q, %v", out, err)
	}
	for step, want := range map[string]string{"split": `"pane_id":"w1:p2"`, "run": `"type":"ok"`} {
		if out, err := exec.Command(bin, "pane", step).Output(); err != nil || !strings.Contains(string(out), want) {
			t.Fatalf("%s: %q, %v", step, out, err)
		}
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if first := strings.Split(string(data), "\n")[0]; first != "pane\x00layout\x00--pane\x00w1:p1\x00" {
		t.Fatalf("first logged call %q", first)
	}
}

func TestScriptFailsAtTheStepAsked(t *testing.T) {
	bin, _ := stub(t, Script(Rect, "split", `{"error":"gone"}`))
	cmd := exec.Command(bin, "pane", "split")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 || len(out) != 0 || stderr.String() != "{\"error\":\"gone\"}\n" {
		t.Fatalf("split: %v, stdout %q, stderr %q", err, out, stderr.String())
	}
	if out, err := exec.Command(bin, "pane", "layout").Output(); err != nil || !strings.Contains(string(out), Rect) {
		t.Fatalf("layout after a failing split: %q, %v", out, err)
	}
}
