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

type Stats struct {
	ExecutionCount      int
	FailedAttemptCount  int
	SuccessCount        int
	FailureCount        int
	RetryCount          int
	RetrieExceededCount int
	AbortCount          int
}

func (s *Stats) Reset() {
	s.ExecutionCount = 0
	s.FailedAttemptCount = 0
	s.SuccessCount = 0
	s.FailureCount = 0
	s.RetryCount = 0
	s.RetrieExceededCount = 0
	s.AbortCount = 0
}

func WithRetryStats[R any](rp retrypolicy.RetryPolicyBuilder[R], stats *Stats) retrypolicy.RetryPolicyBuilder[R] {
	return withRetryStatsAndLogs(rp, stats, false)
}

func WithRetryLogs[R any](rp retrypolicy.RetryPolicyBuilder[R]) retrypolicy.RetryPolicyBuilder[R] {
	return withRetryStatsAndLogs(rp, &Stats{}, true)
}

func WithRetryStatsAndLogs[R any](rp retrypolicy.RetryPolicyBuilder[R], stats *Stats) retrypolicy.RetryPolicyBuilder[R] {
	return withRetryStatsAndLogs(rp, stats, true)
}

func withRetryStatsAndLogs[R any](rp retrypolicy.RetryPolicyBuilder[R], stats *Stats, withLogging bool) retrypolicy.RetryPolicyBuilder[R] {
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
	withStatsAndLogs[retrypolicy.RetryPolicyBuilder[R], R](rp, stats, withLogging)
	return rp
}

func WithBreakerStats[R any](cb circuitbreaker.CircuitBreakerBuilder[R], stats *Stats) circuitbreaker.CircuitBreakerBuilder[R] {
	withBreakerStatsAndLogs(cb, stats, false)
	return cb
}

func WithBreakerLogs[R any](cb circuitbreaker.CircuitBreakerBuilder[R]) circuitbreaker.CircuitBreakerBuilder[R] {
	withBreakerStatsAndLogs(cb, &Stats{}, true)
	return cb
}

func withBreakerStatsAndLogs[R any](cb circuitbreaker.CircuitBreakerBuilder[R], stats *Stats, withLogging bool) circuitbreaker.CircuitBreakerBuilder[R] {
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
	withStatsAndLogs[circuitbreaker.CircuitBreakerBuilder[R], R](cb, stats, withLogging)
	return cb
}

func WithTimeoutStatsAndLogs[R any](to timeout.TimeoutBuilder[R], stats *Stats) timeout.TimeoutBuilder[R] {
	withStatsAndLogs[timeout.TimeoutBuilder[R], R](to, stats, true)
	return to
}

func WithFallbackStatsAndLogs[R any](fb fallback.FallbackBuilder[R], stats *Stats) fallback.FallbackBuilder[R] {
	withStatsAndLogs[fallback.FallbackBuilder[R], R](fb, stats, true)
	return fb
}

func withLogs[P any, R any](policy failsafe.ListenablePolicyBuilder[P, R], stats *Stats, withLogging bool) {
	withStatsAndLogs(policy, &Stats{}, true)
}

func withStatsAndLogs[P any, R any](policy failsafe.ListenablePolicyBuilder[P, R], stats *Stats, withLogging bool) {
	policy.OnSuccess(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.ExecutionCount++
		stats.SuccessCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("%s success [Result: %v, attempts: %d, executions: %d]",
				testutil.GetType(policy), e.Result, e.Attempts, e.Executions))
		}
	})
	policy.OnFailure(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.ExecutionCount++
		stats.FailureCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("%s failure [Result: %v, failure: %s, attempts: %d, executions: %d]",
				testutil.GetType(policy), e.Result, e.Err, e.Attempts, e.Executions))
		}
	})
}
