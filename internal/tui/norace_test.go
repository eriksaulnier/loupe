//go:build !race

package tui

// raceDetector reports whether the test binary runs under the race detector, which slows it several times over.
const raceDetector = false
