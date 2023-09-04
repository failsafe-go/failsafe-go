// Package timeout_testutil is needed to avoid a circular dependency with the spi package.
package timeout_testutil

import (
	"failsafe/internal/testutil"
	"failsafe/timeout"
)

func WithTimeoutStatsAndLogs[R any](to timeout.TimeoutBuilder[R], stats *testutil.Stats) timeout.TimeoutBuilder[R] {
	testutil.WithStatsAndLogs[timeout.TimeoutBuilder[R], R](to, stats, true)
	return to
}
