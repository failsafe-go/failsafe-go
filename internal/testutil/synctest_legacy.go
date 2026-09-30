//go:build !go1.25

package testutil

import (
	"testing"
)

// SyncTest runs f in real time, since synctest requires Go 1.25 or later.
func SyncTest(t *testing.T, f func(t *testing.T)) {
	t.Helper()
	f(t)
}

// SyncRun runs f as a subtest of t with the given name, within a SyncTest.
func SyncRun(t *testing.T, name string, f func(t *testing.T)) bool {
	t.Helper()
	return t.Run(name, func(t *testing.T) {
		SyncTest(t, f)
	})
}
