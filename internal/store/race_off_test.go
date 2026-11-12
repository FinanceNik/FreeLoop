//go:build !race

package store

// raceOn is whether the race detector is on, which makes timings meaningless.
const raceOn = false
