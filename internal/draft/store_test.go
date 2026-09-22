package draft

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

const storeHelperEnv = "LOUPE_TEST_STORE_HELPER"

func TestMain(m *testing.M) {
	if spec := os.Getenv(storeHelperEnv); spec != "" {
		os.Exit(storeHelper(spec))
	}
	os.Exit(m.Run())
}

// storeHelper runs 20 mutations against dir and prints the version each one produced.
func storeHelper(spec string) int {
	dir, name, _ := strings.Cut(spec, "|")
	for i := 0; i < 20; i++ {
		d, err := Mutate(dir, "helper", nil, os.Getenv, func(d *Draft) error {
			d.Replies = append(d.Replies, Reply{ID: NextReplyID(d), Body: fmt.Sprintf("%s-%d", name, i), By: ByAgent})
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(d.Version)
	}
	return 0
}

func newRunDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), NewEmpty()); err != nil {
		t.Fatal(err)
	}
	return dir
}

func noEnv(string) string { return "" }

func TestMutateIncrementsVersion(t *testing.T) {
	dir := newRunDir(t)
	for want := 1; want <= 2; want++ {
		d, err := Mutate(dir, "summary", nil, noEnv, func(d *Draft) error {
			d.Summary += "x"
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if d.Version != want {
			t.Fatalf("returned version %d, want %d", d.Version, want)
		}
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 2 || loaded.Summary != "xx" {
		t.Fatalf("stored %+v", loaded)
	}
}

func TestMutateErrorWritesNothing(t *testing.T) {
	dir := newRunDir(t)
	path := filepath.Join(dir, "draft.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	_, err = Mutate(dir, "add", nil, noEnv, func(d *Draft) error {
		d.Summary = "changed"
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("draft changed:\n%s\n%s", before, after)
	}
}

func TestMutateNoChangeWritesNothing(t *testing.T) {
	dir := newRunDir(t)
	path := filepath.Join(dir, "draft.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Mutate(dir, "edit", nil, noEnv, func(d *Draft) error { return ErrNoChange })
	if err != nil || d == nil || d.Version != 0 {
		t.Fatalf("draft %+v err %v", d, err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("draft changed:\n%s\n%s", before, after)
	}
}

func TestMutateStaleExpectVersion(t *testing.T) {
	dir := newRunDir(t)
	stale := 3
	called := false
	_, err := Mutate(dir, "add", &stale, noEnv, func(*Draft) error {
		called = true
		return nil
	})
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Version || r.Message != "draft is at version 0, expected 3" || r.Fix != "re-read with loupe show --json and retry" {
		t.Fatalf("got %v", err)
	}
	if called {
		t.Fatal("fn must not run on a version mismatch")
	}
	current := 0
	if _, err := Mutate(dir, "add", &current, noEnv, func(*Draft) error { return nil }); err != nil {
		t.Fatalf("matching expectVersion: %v", err)
	}
}

func TestMutateTruncatedDraft(t *testing.T) {
	dir := newRunDir(t)
	path := filepath.Join(dir, "draft.json")
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, data[:len(data)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Mutate(dir, "add", nil, noEnv, func(*Draft) error { return nil })
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Record {
		t.Fatalf("got %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("Load must refuse a truncated draft")
	}
}

func TestConcurrentProcessesMutate(t *testing.T) {
	dir := newRunDir(t)
	names := []string{"a", "b"}
	cmds := make([]*exec.Cmd, len(names))
	outs := make([]*bytes.Buffer, len(names))
	for i, name := range names {
		outs[i] = &bytes.Buffer{}
		cmds[i] = exec.Command(os.Args[0])
		cmds[i].Env = append(os.Environ(), storeHelperEnv+"="+dir+"|"+name)
		cmds[i].Stdout = outs[i]
		cmds[i].Stderr = os.Stderr
		if err := cmds[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	var all []int
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("helper %s: %v", names[i], err)
		}
		prev := 0
		for _, line := range strings.Fields(outs[i].String()) {
			v, err := strconv.Atoi(line)
			if err != nil || v <= prev {
				t.Fatalf("helper %s versions not strictly increasing: %q", names[i], outs[i].String())
			}
			prev = v
			all = append(all, v)
		}
	}
	sort.Ints(all)
	for i, v := range all {
		if v != i+1 {
			t.Fatalf("versions %v are not exactly 1..40", all)
		}
	}
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d.Version != 40 || len(d.Replies) != 40 {
		t.Fatalf("version %d with %d replies", d.Version, len(d.Replies))
	}
	seen := map[string]bool{}
	for i, r := range d.Replies {
		if r.ID != fmt.Sprintf("r-%03d", i+1) || seen[r.Body] {
			t.Fatalf("reply %d: %+v", i, r)
		}
		seen[r.Body] = true
	}
}

func TestLoadRefusesIncompleteDraft(t *testing.T) {
	cases := map[string]string{
		"null":           `null`,
		"empty object":   `{}`,
		"null decisions": `{"schema": 1, "findings": [], "decisions": null, "notes": [], "replies": []}`,
		"missing notes":  `{"schema": 1, "findings": [], "decisions": {}, "replies": []}`,
		"wrong schema":   `{"schema": 2, "findings": [], "decisions": {}, "notes": [], "replies": []}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "draft.json")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir)
			r, ok := refusal.As(err)
			if !ok || r.Code != refusal.Record || !strings.HasPrefix(r.Message, "cannot read "+path+": ") || r.Fix != "inspect it with: cat "+path {
				t.Fatalf("got %v", err)
			}
		})
	}
	if _, err := Load(newRunDir(t)); err != nil {
		t.Fatalf("an empty draft must load: %v", err)
	}
}

func newRunWithFinding(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	d := NewEmpty()
	if _, err := Add(d, []FindingInput{{Title: "Title", Body: "Body.", General: true, Label: "question"}}, nil, ByAgent, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sendBack(dir, body string) (*Draft, error) {
	return Mutate(dir, "review", nil, noEnv, func(d *Draft) error {
		_, err := SendBack(d, "f-001", body, time.Time{})
		return err
	})
}

func TestMutateHandsBackTheNoteASendBackAdds(t *testing.T) {
	dir := newRunWithFinding(t)
	d, err := sendBack(dir, "Why?")
	if err != nil {
		t.Fatal(err)
	}
	if d.Version != 1 {
		t.Fatalf("version %d, want 1: recording the hand-back must not move it", d.Version)
	}
	if _, err := sendBack(dir, "And this?"); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHandBack(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"n-001", "n-002"}; !slices.Equal(h.Notes, want) {
		t.Fatalf("hand-back set %v, want %v", h.Notes, want)
	}
	if got := Awaiting(loadStored(t, dir), h); !slices.Equal(got, []string{"n-001", "n-002"}) {
		t.Fatalf("awaiting %v", got)
	}
}

func TestMutateWithoutANewNoteLeavesNoHandBack(t *testing.T) {
	dir := newRunWithFinding(t)
	if _, err := Mutate(dir, "review", nil, noEnv, func(d *Draft) error {
		_, err := Accept(d, "f-001", time.Time{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "handback.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("handback.json must not exist: %v", err)
	}
}

func TestMutateThatFailsAfterASendBackWritesNeitherFile(t *testing.T) {
	dir := newRunWithFinding(t)
	failed := errors.New("later step failed")
	_, err := Mutate(dir, "review", nil, noEnv, func(d *Draft) error {
		if _, err := SendBack(d, "f-001", "Why?", time.Time{}); err != nil {
			return err
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "handback.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("handback.json must not exist: %v", err)
	}
	if d := loadStored(t, dir); len(d.Notes) != 0 || d.Version != 0 {
		t.Fatalf("draft changed: version %d, notes %v", d.Version, d.Notes)
	}
}

func loadStored(t *testing.T, dir string) *Draft {
	t.Helper()
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
