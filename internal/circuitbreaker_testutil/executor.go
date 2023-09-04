// Package circuitbreaker_testutil is needed to avoid a circular dependency with the spi package.
package circuitbreaker_testutil

import (
	"fmt"

	"failsafe/circuitbreaker"
	"failsafe/internal/testutil"
)

func WithBreakerStats[R any](cb circuitbreaker.CircuitBreakerBuilder[R], stats *testutil.Stats) circuitbreaker.CircuitBreakerBuilder[R] {
	withBreakerStatsAndLogs(cb, stats, false)
	return cb
}

func WithBreakerLogs[R any](cb circuitbreaker.CircuitBreakerBuilder[R]) circuitbreaker.CircuitBreakerBuilder[R] {
	withBreakerStatsAndLogs(cb, &testutil.Stats{}, true)
	return cb
}

func WithBreakerStatsAndLogs[R any](cb circuitbreaker.CircuitBreakerBuilder[R], stats *testutil.Stats) circuitbreaker.CircuitBreakerBuilder[R] {
	return withBreakerStatsAndLogs(cb, stats, true)
}

func withBreakerStatsAndLogs[R any](cb circuitbreaker.CircuitBreakerBuilder[R], stats *testutil.Stats, withLogging bool) circuitbreaker.CircuitBreakerBuilder[R] {
	cb.OnOpen(func(event circuitbreaker.StateChangedEvent) {
		if withLogging {
			fmt.Println(fmt.Sprintf("CircuitBreaker %p opened", cb))
		}
	})
	cb.OnHalfOpen(func(event circuitbreaker.StateChangedEvent) {
		if withLogging {
			fmt.Println(fmt.Sprintf("CircuitBreaker %p half-opened", cb))
		}
	})
	cb.OnClose(func(event circuitbreaker.StateChangedEvent) {
		if withLogging {
			fmt.Println(fmt.Sprintf("CircuitBreaker %p closed", cb))
		}
	})
	testutil.WithStatsAndLogs[circuitbreaker.CircuitBreakerBuilder[R], R](cb, stats, withLogging)
	return cb
}
