//go:build !race

package cli

// raceDetector reports whether the test binary runs under the race detector, which slows it several times over.
const raceDetector = false
