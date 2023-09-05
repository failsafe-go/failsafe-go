// Package policytesting is needed to avoid a circular dependency with the spi package.
package policytesting

import (
	"fmt"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

func WithRetryStats[R any](rp retrypolicy.RetryPolicyBuilder[R], stats *testutil.Stats) retrypolicy.RetryPolicyBuilder[R] {
	return withRetryStatsAndLogs(rp, stats, false)
}

func WithRetryLogs[R any](rp retrypolicy.RetryPolicyBuilder[R]) retrypolicy.RetryPolicyBuilder[R] {
	return withRetryStatsAndLogs(rp, &testutil.Stats{}, true)
}

func WithRetryStatsAndLogs[R any](rp retrypolicy.RetryPolicyBuilder[R], stats *testutil.Stats) retrypolicy.RetryPolicyBuilder[R] {
	return withRetryStatsAndLogs(rp, stats, true)
}

func withRetryStatsAndLogs[R any](rp retrypolicy.RetryPolicyBuilder[R], stats *testutil.Stats, withLogging bool) retrypolicy.RetryPolicyBuilder[R] {
	rp.OnFailedAttempt(func(e failsafe.ExecutionAttemptedEvent[R]) {
		stats.ExecutionCount++
		stats.FailedAttemptCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("RetryPolicy %p failed attempt [Result: %v, failure: %s, attempts: %d, executions: %d]",
				rp, e.LastResult, e.LastErr, e.Attempts, e.Executions))
		}
	}).OnRetry(func(e failsafe.ExecutionAttemptedEvent[R]) {
		stats.RetryCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("RetryPolicy %p retrying [Result: %v, failure: %s]", rp, e.LastResult, e.LastErr))
		}
	}).OnRetriesExceeded(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.RetrieExceededCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("RetryPolicy %p retries exceeded", rp))
		}
	}).OnAbort(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.AbortCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("RetryPolicy %p abort", rp))
		}
	})
	testutil.WithStatsAndLogs[retrypolicy.RetryPolicyBuilder[R], R](rp, stats, withLogging)
	return rp
}

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

func WithTimeoutStatsAndLogs[R any](to timeout.TimeoutBuilder[R], stats *testutil.Stats) timeout.TimeoutBuilder[R] {
	testutil.WithStatsAndLogs[timeout.TimeoutBuilder[R], R](to, stats, true)
	return to
}

func WithFallbackStatsAndLogs[R any](fb fallback.FallbackBuilder[R], stats *testutil.Stats) fallback.FallbackBuilder[R] {
	testutil.WithStatsAndLogs[fallback.FallbackBuilder[R], R](fb, stats, true)
	return fb
}
