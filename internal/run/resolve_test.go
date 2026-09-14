package run

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRoundsAndNewest(t *testing.T) {
	root := t.TempDir()
	prDir := filepath.Dir(RunDir(root, "o", "r", 5, 1))
	mkdirs(t, RunDir(root, "o", "r", 5, 2), RunDir(root, "o", "r", 5, 10), RunDir(root, "o", "r", 5, 1), filepath.Join(prDir, ".3.tmp-123"))
	for _, name := range []string{"7", ".lock"} {
		if err := os.WriteFile(filepath.Join(prDir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rounds, err := Rounds(root, "o", "r", 5)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rounds, []int{1, 2, 10}) {
		t.Fatalf("got %v", rounds)
	}
	if n, err := Newest(root, "o", "r", 5); err != nil || n != 10 {
		t.Fatalf("Newest: %d, %v", n, err)
	}
	if rounds, err := Rounds(root, "o", "r", 6); err != nil || len(rounds) != 0 {
		t.Fatalf("no pull request directory: %v, %v", rounds, err)
	}
	if n, err := Newest(root, "o", "r", 6); err != nil || n != 0 {
		t.Fatalf("Newest without rounds: %d, %v", n, err)
	}
}

func TestResolveRef(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, RunDir(root, "o", "r", 5, 1), RunDir(root, "o", "r", 5, 2))

	dir, resolved, err := ResolveRef(root, Ref{Owner: "o", Repo: "r", Number: 5})
	if err != nil || dir != RunDir(root, "o", "r", 5, 2) || resolved.Round != 2 {
		t.Fatalf("newest: %s %+v %v", dir, resolved, err)
	}
	dir, resolved, err = ResolveRef(root, Ref{Owner: "o", Repo: "r", Number: 5, Round: 1})
	if err != nil || dir != RunDir(root, "o", "r", 5, 1) || resolved.Round != 1 {
		t.Fatalf("explicit: %s %+v %v", dir, resolved, err)
	}

	for _, ref := range []Ref{{Owner: "o", Repo: "r", Number: 5, Round: 3}, {Owner: "o", Repo: "r", Number: 9}} {
		_, _, err := ResolveRef(root, ref)
		r, ok := refusal.As(err)
		if !ok || r.Code != refusal.NoRun {
			t.Fatalf("%s: got %v", ref, err)
		}
		want := "loupe capture https://github.com/o/r/pull/" + strconv.Itoa(ref.Number) + " or --run <ref>"
		if r.Fix != want {
			t.Fatalf("%s: fix %q, want %q", ref, r.Fix, want)
		}
	}
}

func TestHasReceipt(t *testing.T) {
	dir := t.TempDir()
	if ok, err := HasReceipt(dir); err != nil || ok {
		t.Fatalf("receipt: %v %v", ok, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "receipt.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := HasReceipt(dir); err != nil || !ok {
		t.Fatalf("receipt: %v %v", ok, err)
	}
}
