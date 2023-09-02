package cbtesting

import (
	"failsafe/internal/testutil"

	"failsafe/circuitbreaker"
)

func WithBreakerStats[R any](cb circuitbreaker.CircuitBreakerBuilder[R], stats *testutil.Stats) circuitbreaker.CircuitBreakerBuilder[R] {
	WithBreakerStatsAndLogs(cb, stats, false)
	return cb
}

func WithBreakerStatsAndLogs[R any](cb circuitbreaker.CircuitBreakerBuilder[R], stats *testutil.Stats, withLogging bool) circuitbreaker.CircuitBreakerBuilder[R] {
	testutil.WithStatsAndLogs[circuitbreaker.CircuitBreakerBuilder[R], R](cb, stats, withLogging)
	return cb
}
