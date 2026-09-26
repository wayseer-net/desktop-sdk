//go:build !race

package sdktest

// slowdown stretches waits, since the race detector slows code several times over.
const slowdown = 1
