// Package policytesting is needed to avoid a circular dependency with the policy package.
package policytesting

import (
	"fmt"
	"reflect"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

type Stats struct {
	ExecutionCount int
	SuccessCount   int
	FailureCount   int

	// Retry specific stats
	RetryCount          int
	RetrieExceededCount int
	AbortCount          int
}

func (s *Stats) Reset() {
	s.ExecutionCount = 0
	s.SuccessCount = 0
	s.FailureCount = 0

	// Retry specific stats
	s.RetryCount = 0
	s.RetrieExceededCount = 0
	s.AbortCount = 0
}

func ResetRateLimiter[R any](cb ratelimiter.RateLimiter[R]) {
	cbElem := reflect.ValueOf(cb)
	resetMethod := cbElem.MethodByName("Reset")
	resetMethod.Call([]reflect.Value{})
}

func ResetCircuitBreaker[R any](cb circuitbreaker.CircuitBreaker[R]) {
	cbElem := reflect.ValueOf(cb)
	resetMethod := cbElem.MethodByName("Reset")
	resetMethod.Call([]reflect.Value{})
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
	rp.OnRetry(func(e failsafe.ExecutionEvent[R]) {
		stats.RetryCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("%s %p retrying [result: %v, error: %s]", testutil.GetType(rp), rp, e.LastResult(), e.LastError()))
		}
	}).OnRetriesExceeded(func(e failsafe.ExecutionEvent[R]) {
		stats.RetrieExceededCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("%s %p retries exceeded", testutil.GetType(rp), rp))
		}
	}).OnAbort(func(e failsafe.ExecutionEvent[R]) {
		stats.AbortCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("%s %p abort", testutil.GetType(rp), rp))
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
			fmt.Println(fmt.Sprintf("%s %p opened", testutil.GetType(cb), cb))
		}
	})
	cb.OnHalfOpen(func(event circuitbreaker.StateChangedEvent) {
		if withLogging {
			fmt.Println(fmt.Sprintf("%s %p half-opened", testutil.GetType(cb), cb))
		}
	})
	cb.OnClose(func(event circuitbreaker.StateChangedEvent) {
		if withLogging {
			fmt.Println(fmt.Sprintf("%s %p closed", testutil.GetType(cb), cb))
		}
	})
	withStatsAndLogs[circuitbreaker.CircuitBreakerBuilder[R], R](cb, stats, withLogging)
	return cb
}

func WithTimeoutStatsAndLogs[R any](to timeout.TimeoutBuilder[R], stats *Stats) timeout.TimeoutBuilder[R] {
	to.OnTimeoutExceeded(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.ExecutionCount++
		fmt.Println(fmt.Sprintf("%s %p exceeded [attempts: %d, executions: %d]",
			testutil.GetType(to), to, e.Attempts(), e.Executions()))
	})
	return to
}

func WithFallbackStatsAndLogs[R any](fb fallback.FallbackBuilder[R], stats *Stats) fallback.FallbackBuilder[R] {
	fb.OnFallbackExecuted(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.ExecutionCount++
		fmt.Println(fmt.Sprintf("%s %p complete [result: %v, error: %s, attempts: %d, executions: %d]",
			testutil.GetType(fb), fb, e.Result, e.Error, e.Attempts(), e.Executions()))
	})
	return fb
}

func withStatsAndLogs[P any, R any](policy failsafe.FailurePolicyBuilder[P, R], stats *Stats, withLogging bool) {
	policy.OnSuccess(func(e failsafe.ExecutionEvent[R]) {
		stats.ExecutionCount++
		stats.SuccessCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("%s %p success [result: %v, attempts: %d, executions: %d]",
				testutil.GetType(policy), policy, e.LastResult(), e.Attempts(), e.Executions()))
		}
	})
	policy.OnFailure(func(e failsafe.ExecutionEvent[R]) {
		stats.ExecutionCount++
		stats.FailureCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("%s %p failure [result: %v, error: %s, attempts: %d, executions: %d]",
				testutil.GetType(policy), policy, e.LastResult(), e.LastError(), e.Attempts(), e.Executions()))
		}
	})
}
