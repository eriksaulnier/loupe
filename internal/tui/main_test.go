package tui

import (
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	settleAfterDecision = 0
	settleOnOpen = 0
	// Poll tests deliver the tick themselves; a real one would race them.
	pollInterval = time.Hour
	// The frame's clip would hide a view that draws wider than the window from every width check in the package.
	clipToWindow = false
	os.Exit(m.Run())
}
