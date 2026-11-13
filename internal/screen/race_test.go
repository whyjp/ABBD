//go:build race

package screen

// raceOn is whether the race detector runs, which slows a build many times
// over, so wall-time bounds don't hold under it.
const raceOn = true
