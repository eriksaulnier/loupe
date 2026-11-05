package run

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
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

func TestResolveLocalRef(t *testing.T) {
	root := t.TempDir()
	target := sampleLocalTarget()
	target.Local.Prefix = "myrepo"
	dir, ref, err := NextLocal(root, "test", os.Getenv, target, nil, sampleDraftJSON(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// prefix from input is ignored; prefix from stored target is returned
	for _, inputRef := range []Ref{{Local: ref.Local}, {Local: ref.Local, Prefix: "wrong"}} {
		gotDir, gotRef, err := ResolveRef(root, inputRef)
		if err != nil {
			t.Fatalf("%v: %v", inputRef, err)
		}
		if gotDir != dir {
			t.Fatalf("%v: dir %q, want %q", inputRef, gotDir, dir)
		}
		if gotRef.Local != ref.Local || gotRef.Prefix != "myrepo" {
			t.Fatalf("%v: ref %+v, want Local=%d Prefix=myrepo", inputRef, gotRef, ref.Local)
		}
	}
}

func TestResolveLocalRefMissingRefusesNoRun(t *testing.T) {
	root := t.TempDir()
	_, _, err := ResolveRef(root, Ref{Local: 99})
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.NoRun {
		t.Fatalf("got %v", err)
	}
	if r.Fix != "loupe list" {
		t.Fatalf("fix %q, want %q", r.Fix, "loupe list")
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

// A pull request or local runs directory that is a file, not a directory, is an error to report, not a missing run.
func TestResolveRefReturnsAnUnreadableRunDirectory(t *testing.T) {
	root := t.TempDir()
	prDir := filepath.Dir(RunDir(root, "o", "r", 5, 1))
	mkdirs(t, filepath.Dir(prDir))
	if err := os.WriteFile(prDir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LocalRoot(root), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		ref    Ref
		prefix string
	}{
		{"newest round", Ref{Owner: "o", Repo: "r", Number: 5}, "list " + prDir},
		{"given round", Ref{Owner: "o", Repo: "r", Number: 5, Round: 1}, "stat run "},
		{"local run", Ref{Local: 1}, "stat local run "},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := ResolveRef(root, c.ref); !errors.Is(err, syscall.ENOTDIR) || !strings.HasPrefix(err.Error(), c.prefix) {
				t.Fatalf("got %v, want ENOTDIR starting %q", err, c.prefix)
			}
		})
	}
}
