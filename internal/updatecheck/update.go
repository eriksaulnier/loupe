// Package updatecheck tells a person running an old release that a newer one exists. The check is a convenience, so
// nothing here returns an error: a failed lookup or an unreadable cache means loupe says nothing.
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/secondshift-labs/loupe/internal/run"
)

const (
	// Owner and Repo name the repository whose latest release is checked.
	Owner = "secondshift-labs"
	Repo  = "loupe"
	// Command is what a warning tells a person to run. It refuses an install it cannot update, with that install's fix.
	Command = "loupe update"
	// CheckInterval is the least time between lookups.
	CheckInterval = 24 * time.Hour
	// LookupTimeout is the most a lookup may add to a command.
	LookupTimeout = 2 * time.Second
	// DisableEnv, when set, stops the check, the warning and the notice.
	DisableEnv = "LOUPE_NO_UPDATE_CHECK"
)

// Lookup returns the tag of the latest release.
type Lookup func(ctx context.Context) (string, error)

type cache struct {
	CheckedAt time.Time `json:"checkedAt"`
	Latest    string    `json:"latest"`
}

// CachePath is a file at the data root, outside the runs directory that run.Walk lists.
func CachePath(root string) string {
	return filepath.Join(root, "update-check.json")
}

// Enabled is false for a build that is not a release and for a CI job, which pins its version on purpose.
func Enabled(current string, getenv func(string) string) bool {
	if _, ok := parse(current); !ok {
		return false
	}
	return getenv("CI") == "" && getenv(DisableEnv) == ""
}

// IsRelease says that version is X.Y.Z, which only a release build carries.
func IsRelease(version string) bool {
	_, ok := parse(version)
	return ok
}

// Cached returns the latest version from the last check, or nothing.
func Cached(path string) string {
	return read(path).Latest
}

// Refresh looks up the latest release unless a check ran within CheckInterval. A failed lookup also counts as a check,
// so that an offline machine pays LookupTimeout at most once per interval.
func Refresh(ctx context.Context, path string, now time.Time, lookup Lookup) {
	c := read(path)
	if age := now.Sub(c.CheckedAt); age >= 0 && age < CheckInterval {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, LookupTimeout)
	defer cancel()
	if tag, err := lookup(ctx); err == nil {
		c.Latest = strings.TrimPrefix(tag, "v")
	}
	c.CheckedAt = now
	// A cache that cannot be written only means another lookup next time.
	_ = run.WriteJSONAtomic(path, c)
}

// Available says that latest is newer than current, or returns nothing. It compares when read, not when cached, so an
// upgrade clears it at once.
func Available(current, latest string) string {
	have, ok := parse(current)
	if !ok {
		return ""
	}
	want, ok := parse(latest)
	if !ok {
		return ""
	}
	for i := range have {
		if want[i] != have[i] {
			if want[i] < have[i] {
				return ""
			}
			return fmt.Sprintf("loupe %s is available, you have %s", latest, current)
		}
	}
	return ""
}

func read(path string) cache {
	var c cache
	data, err := os.ReadFile(path)
	if err != nil {
		return cache{}
	}
	if json.Unmarshal(data, &c) != nil {
		return cache{}
	}
	return c
}

// parse accepts only X.Y.Z, which is what a release stamps. A dev build and git describe output are not releases.
func parse(version string) ([3]uint64, bool) {
	var out [3]uint64
	parts := strings.Split(version, ".")
	if len(parts) != len(out) {
		return out, false
	}
	for i, part := range parts {
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
