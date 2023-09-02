package cbtesting

import (
	"fmt"

	"failsafe/internal/testutil"

	"failsafe/circuitbreaker"
)

func WithBreakerStats[R any](cb circuitbreaker.CircuitBreakerBuilder[R], stats *testutil.Stats) circuitbreaker.CircuitBreakerBuilder[R] {
	WithBreakerStatsAndLogs(cb, stats, false)
	return cb
}

func WithBreakerLogs[R any](cb circuitbreaker.CircuitBreakerBuilder[R]) circuitbreaker.CircuitBreakerBuilder[R] {
	WithBreakerStatsAndLogs(cb, &testutil.Stats{}, true)
	return cb
}

func WithBreakerStatsAndLogs[R any](cb circuitbreaker.CircuitBreakerBuilder[R], stats *testutil.Stats, withLogging bool) circuitbreaker.CircuitBreakerBuilder[R] {
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
