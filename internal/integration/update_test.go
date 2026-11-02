package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/updatecheck"
)

const releasePath = "/repos/" + updatecheck.Owner + "/" + updatecheck.Repo + "/releases/latest"

func (h *harness) releaseLookups() int {
	h.t.Helper()
	n := 0
	for _, r := range h.GH.Requests() {
		if r.Method == "GET" && r.Path == releasePath {
			n++
		}
	}
	return n
}

// seedUpdateCache writes the cache as of the harness's clock, so a capture counts it as fresh.
func (h *harness) seedUpdateCache(latest string) {
	h.t.Helper()
	if err := os.MkdirAll(h.Home, 0o755); err != nil {
		h.t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"checkedAt": fixedNow(), "latest": latest})
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(updatecheck.CachePath(h.Home), data, 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func decodeResult(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	return result
}

func TestCaptureWarnsWhenANewerReleaseExists(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.Version = "0.19.0"
	h.GH.SetLatestRelease("v0.20.0")

	stdout, stderr, exit := h.Run("capture", prURL(), "--json")
	if exit != 0 {
		t.Fatalf("exit %d: %s", exit, stderr)
	}
	want := "warning: loupe 0.20.0 is available, you have 0.19.0. To update, see "
	if !strings.Contains(stderr, want) || !strings.Contains(stderr, "docs/install.md") {
		t.Errorf("stderr = %q", stderr)
	}
	warnings, _ := decodeResult(t, stdout)["warnings"].([]any)
	if len(warnings) != 1 || warnings[0] != "update-available" {
		t.Errorf("warnings = %v", warnings)
	}
	if got := updatecheck.Cached(updatecheck.CachePath(h.Home)); got != "0.20.0" {
		t.Errorf("Cached = %q", got)
	}
}

func TestCaptureSaysNothingWithoutANewerRelease(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.Version = "0.20.0"
	h.GH.SetLatestRelease("v0.20.0")

	stdout, stderr, exit := h.Run("capture", prURL(), "--json")
	if exit != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", exit, stderr)
	}
	if _, found := decodeResult(t, stdout)["warnings"]; found {
		t.Errorf("result carries warnings: %s", stdout)
	}
}

func TestCaptureSurvivesAFailedLookup(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.Version = "0.19.0"
	h.GH.Fail("GET", releasePath, 500)

	if _, stderr, exit := h.Run("capture", prURL(), "--json"); exit != 0 || stderr != "" {
		t.Errorf("exit %d, stderr %q", exit, stderr)
	}
	if got := h.releaseLookups(); got != 1 {
		t.Errorf("lookups = %d, want 1", got)
	}
}

func TestCaptureSurvivesAStalledLookup(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.Version = "0.19.0"
	h.GH.StallLatestRelease()

	start := time.Now()
	_, stderr, exit := h.Run("capture", prURL(), "--json")
	if exit != 0 || stderr != "" {
		t.Errorf("exit %d, stderr %q", exit, stderr)
	}
	if got := h.releaseLookups(); got != 1 {
		t.Errorf("lookups = %d, want 1", got)
	}
	// TestRefreshGivesUpOnAStalledLookup pins the limit, so this checks only the lower bound.
	if elapsed := time.Since(start); elapsed < updatecheck.LookupTimeout {
		t.Errorf("capture took %v, so it did not wait for the stalled lookup", elapsed)
	}
}

func TestCaptureUsesAFreshCacheWithoutALookup(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.Version = "0.19.0"
	h.seedUpdateCache("0.20.0")

	_, stderr, exit := h.Run("capture", prURL(), "--json")
	if exit != 0 {
		t.Fatalf("exit %d: %s", exit, stderr)
	}
	if got := h.releaseLookups(); got != 0 {
		t.Errorf("lookups = %d, want 0", got)
	}
	if !strings.Contains(stderr, "warning: loupe 0.20.0 is available") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestCaptureSkipsTheCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		arrange func(h *harness)
	}{
		{"a dev build", func(h *harness) { h.Version = "" }},
		{"LOUPE_NO_UPDATE_CHECK", func(h *harness) { h.Env["LOUPE_NO_UPDATE_CHECK"] = "1" }},
		{"CI", func(h *harness) { h.Env["CI"] = "true" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.Version = "0.19.0"
			h.GH.SetLatestRelease("v0.20.0")
			tc.arrange(h)

			_, stderr, exit := h.Run("capture", prURL(), "--json")
			if exit != 0 || stderr != "" {
				t.Errorf("exit %d, stderr %q", exit, stderr)
			}
			if got := h.releaseLookups(); got != 0 {
				t.Errorf("lookups = %d, want 0", got)
			}
		})
	}
}

func TestCaptureMakesNoLookupWithAnInstallationToken(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.Version = "0.19.0"
	h.GH.SetLatestRelease("v0.20.0")
	h.UseInstallationToken()

	if _, stderr, exit := h.Run("capture", prURL(), "--json"); exit != 0 {
		t.Fatalf("exit %d: %s", exit, stderr)
	}
	if got := h.releaseLookups(); got != 0 {
		t.Errorf("lookups = %d, want 0", got)
	}
}

func TestRefusedCaptureMakesNoLookup(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.Version = "0.19.0"
	h.GH.SetLatestRelease("v0.20.0")
	h.setPRState("closed")

	_, stderr, exit := h.Run("capture", prURL(), "--json")
	if exit == 0 {
		t.Fatal("capture of a closed pull request exited 0")
	}
	if strings.Contains(stderr, "warning:") || h.releaseLookups() != 0 {
		t.Errorf("stderr %q, lookups %d", stderr, h.releaseLookups())
	}
}

func TestUpdateCacheLeavesTheDataRootReadersAlone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.seedUpdateCache("0.20.0")

	for _, command := range []string{"list", "stats"} {
		stdout, stderr, exit := h.Run(command, "--json")
		if exit != 0 {
			t.Fatalf("%s exit %d: %s", command, exit, stderr)
		}
		if command == "list" && !strings.Contains(stdout, runRef) {
			t.Errorf("list does not show the run: %s", stdout)
		}
	}
	if _, err := os.Stat(filepath.Join(h.Home, "update-check.json")); err != nil {
		t.Errorf("the cache is not at the data root: %v", err)
	}
}

func TestReviewShowsTheCachedUpdate(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.Version = "0.19.0"
	h.seedUpdateCache("0.20.0")
	before := len(h.GH.Requests())

	s := h.Start("review", runRef)
	s.WaitFor("loupe 0.20.0 is available, you have 0.19.0: see docs/install.md")
	s.Press("q")
	if _, stderr, exit := s.Wait(); exit != 0 {
		t.Fatalf("exit %d: %s", exit, stderr)
	}
	if got := len(h.GH.Requests()); got != before {
		t.Errorf("review made %d GitHub requests, want none", got-before)
	}
}

func TestReviewShowsNoNoticeWhenCurrent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		arrange func(h *harness)
	}{
		{"no cache", func(*harness) {}},
		{"the latest release", func(h *harness) { h.seedUpdateCache("0.19.0") }},
		{"turned off", func(h *harness) {
			h.seedUpdateCache("0.20.0")
			h.Env["LOUPE_NO_UPDATE_CHECK"] = "1"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.capture()
			h.Version = "0.19.0"
			tc.arrange(h)

			s := h.Start("review", runRef)
			view := s.WaitFor("Add widgets")
			if strings.Contains(view, "is available") {
				t.Errorf("review shows a notice:\n%s", view)
			}
			s.Press("q")
			if _, stderr, exit := s.Wait(); exit != 0 {
				t.Fatalf("exit %d: %s", exit, stderr)
			}
		})
	}
}
