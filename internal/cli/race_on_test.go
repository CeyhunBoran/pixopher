//go:build race

package cli

// raceEnabled reports whether the race detector is on; it slows commands
// down several times, so timing checks skip.
const raceEnabled = true
