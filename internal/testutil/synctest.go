//go:build go1.25

package testutil

import (
	"testing"
	"testing/synctest"
	"time"
)

// SyncTest runs f in a synctest bubble, where time is virtual and advances instantly whenever every goroutine in the
// bubble is blocked. This lets tests that wait on sleeps, timers, and timeouts complete without waiting in real time.
// On Go versions before 1.25, f runs normally in real time.
//
// Everything that f waits on, including policies and executors, should be created inside f. After f returns, goroutines
// that are still sleeping, such as blocked executions that ignore cancellation, are allowed to finish in virtual time.
// Goroutines that remain blocked forever cause the test to fail.
func SyncTest(t *testing.T, f func(t *testing.T)) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		f(t)
		time.Sleep(time.Hour)
	})
}

// SyncRun runs f as a subtest of t with the given name, within a SyncTest.
func SyncRun(t *testing.T, name string, f func(t *testing.T)) bool {
	t.Helper()
	return t.Run(name, func(t *testing.T) {
		SyncTest(t, f)
	})
}
