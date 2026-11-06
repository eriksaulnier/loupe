package updatecheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var checkTime = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

func TestEnabled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current string
		env     map[string]string
		want    bool
	}{
		{name: "a release", current: "0.19.0", want: true},
		{name: "a dev build", current: "dev"},
		{name: "a build from git describe", current: "v0.19.0-3-gabc1234"},
		{name: "CI", current: "0.19.0", env: map[string]string{"CI": "true"}},
		{name: "turned off", current: "0.19.0", env: map[string]string{"LOUPE_NO_UPDATE_CHECK": "1"}},
		{name: "empty values count as unset", current: "0.19.0", env: map[string]string{"CI": "", "LOUPE_NO_UPDATE_CHECK": ""}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Enabled(tc.current, func(k string) string { return tc.env[k] }); got != tc.want {
				t.Errorf("Enabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAvailable(t *testing.T) {
	for _, tc := range []struct{ current, latest, want string }{
		{"0.19.0", "0.20.0", "loupe 0.20.0 is available, you have 0.19.0"},
		{"0.9.0", "0.10.0", "loupe 0.10.0 is available, you have 0.9.0"},
		{"1.2.3", "1.2.4", "loupe 1.2.4 is available, you have 1.2.3"},
		{"1.9.9", "2.0.0", "loupe 2.0.0 is available, you have 1.9.9"},
		{"0.20.0", "0.20.0", ""},
		{"0.21.0", "0.20.0", ""},
		{"0.19.0", "", ""},
		{"0.19.0", "garbage", ""},
		{"0.19.0", "1.2", ""},
		{"0.19.0", "1.2.-3", ""},
		{"dev", "0.20.0", ""},
	} {
		if got := Available(tc.current, tc.latest); got != tc.want {
			t.Errorf("Available(%q, %q) = %q, want %q", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestCachedIsEmptyWhenTheFileIsMissingOrCorrupt(t *testing.T) {
	path := CachePath(t.TempDir())
	if got := Cached(path); got != "" {
		t.Errorf("missing file: Cached = %q", got)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Cached(path); got != "" {
		t.Errorf("corrupt file: Cached = %q", got)
	}
}

func TestRefreshStoresTheVersionWithoutItsPrefix(t *testing.T) {
	path := CachePath(t.TempDir())
	Refresh(context.Background(), path, checkTime, func(context.Context) (string, error) { return "v0.20.0", nil })
	if got := Cached(path); got != "0.20.0" {
		t.Errorf("Cached = %q, want 0.20.0", got)
	}
}

func TestRefreshLooksUpAtMostOncePerInterval(t *testing.T) {
	path := CachePath(t.TempDir())
	calls := 0
	lookup := func(context.Context) (string, error) { calls++; return "v0.20.0", nil }

	Refresh(context.Background(), path, checkTime, lookup)
	Refresh(context.Background(), path, checkTime.Add(CheckInterval-time.Second), lookup)
	if calls != 1 {
		t.Fatalf("lookups inside the interval = %d, want 1", calls)
	}
	Refresh(context.Background(), path, checkTime.Add(CheckInterval), lookup)
	if calls != 2 {
		t.Errorf("lookups after the interval = %d, want 2", calls)
	}
}

// A clock set back MUST NOT silence the check until the clock catches up.
func TestRefreshLooksUpWhenTheLastCheckIsInTheFuture(t *testing.T) {
	path := CachePath(t.TempDir())
	calls := 0
	lookup := func(context.Context) (string, error) { calls++; return "v0.20.0", nil }
	Refresh(context.Background(), path, checkTime, lookup)
	Refresh(context.Background(), path, checkTime.Add(-time.Hour), lookup)
	if calls != 2 {
		t.Errorf("lookups = %d, want 2", calls)
	}
}

func TestRefreshKeepsTheVersionAndRecordsTheTimeWhenTheLookupFails(t *testing.T) {
	path := CachePath(t.TempDir())
	Refresh(context.Background(), path, checkTime, func(context.Context) (string, error) { return "v0.20.0", nil })

	later := checkTime.Add(CheckInterval)
	calls := 0
	failing := func(context.Context) (string, error) { calls++; return "", errors.New("offline") }
	Refresh(context.Background(), path, later, failing)
	if got := Cached(path); got != "0.20.0" {
		t.Errorf("Cached = %q, want the earlier version", got)
	}
	Refresh(context.Background(), path, later.Add(time.Hour), failing)
	if calls != 1 {
		t.Errorf("failed lookups = %d, want 1 per interval", calls)
	}
}

func TestRefreshBoundsTheLookup(t *testing.T) {
	path := CachePath(t.TempDir())
	Refresh(context.Background(), path, checkTime, func(ctx context.Context) (string, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("the lookup has no deadline")
		}
		if left := time.Until(deadline); left > LookupTimeout || left < LookupTimeout-time.Second {
			t.Errorf("the lookup may run for %v, want about %v", left, LookupTimeout)
		}
		return "", errors.New("offline")
	})
}

// 048 FR-002 fixes the limit at 2 seconds.
func TestLookupTimeoutIsTwoSeconds(t *testing.T) {
	if LookupTimeout != 2*time.Second {
		t.Fatalf("LookupTimeout = %v, want 2s", LookupTimeout)
	}
}

func TestRefreshReplacesACorruptCache(t *testing.T) {
	path := CachePath(t.TempDir())
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	Refresh(context.Background(), path, checkTime, func(context.Context) (string, error) { return "v0.20.0", nil })
	if got := Cached(path); got != "0.20.0" {
		t.Errorf("Cached = %q, want 0.20.0", got)
	}
}

func TestRefreshIgnoresAnUnwritableCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "update-check.json")
	Refresh(context.Background(), path, checkTime, func(context.Context) (string, error) { return "v0.20.0", nil })
	if got := Cached(path); got != "" {
		t.Errorf("Cached = %q, want nothing", got)
	}
}

func TestIsRelease(t *testing.T) {
	for version, want := range map[string]bool{"0.19.0": true, "dev": false, "0.22.0-rc.1": false, "v0.19.0-3-gabc1234": false} {
		if got := IsRelease(version); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", version, got, want)
		}
	}
}
