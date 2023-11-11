// Package policytesting is needed to avoid a circular dependency with the policy package.
package policytesting

import (
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

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
		stats.retryCount.Add(1)
		if withLogging {
			fmt.Printf("%s %p retrying [result: %v, error: %s]\n", testutil.GetType(rp), rp, e.LastResult(), e.LastError())
		}
	}).OnRetriesExceeded(func(e failsafe.ExecutionEvent[R]) {
		stats.retriesExceededCount.Add(1)
		if withLogging {
			fmt.Printf("%s %p retries exceeded\n", testutil.GetType(rp), rp)
		}
	}).OnAbort(func(e failsafe.ExecutionEvent[R]) {
		stats.abortCount.Add(1)
		if withLogging {
			fmt.Printf("%s %p abort\n", testutil.GetType(rp), rp)
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
			fmt.Printf("%s %p opened\n", testutil.GetType(cb), cb)
		}
	})
	cb.OnHalfOpen(func(event circuitbreaker.StateChangedEvent) {
		if withLogging {
			fmt.Printf("%s %p half-opened\n", testutil.GetType(cb), cb)
		}
	})
	cb.OnClose(func(event circuitbreaker.StateChangedEvent) {
		if withLogging {
			fmt.Printf("%s %p closed\n", testutil.GetType(cb), cb)
		}
	})
	withStatsAndLogs[circuitbreaker.CircuitBreakerBuilder[R], R](cb, stats, withLogging)
	return cb
}

func WithTimeoutStatsAndLogs[R any](to timeout.TimeoutBuilder[R], stats *Stats) timeout.TimeoutBuilder[R] {
	to.OnTimeoutExceeded(func(e failsafe.ExecutionDoneEvent[R]) {
		stats.executionCount.Add(1)
		fmt.Printf("%s %p exceeded [attempts: %d, executions: %d]\n", testutil.GetType(to), to, e.Attempts(), e.Executions())
	})
	return to
}

func WithFallbackStatsAndLogs[R any](fb fallback.FallbackBuilder[R], stats *Stats) fallback.FallbackBuilder[R] {
	fb.OnFallbackExecuted(func(e failsafe.ExecutionDoneEvent[R]) {
		stats.executionCount.Add(1)
		fmt.Printf("%s %p done [result: %v, error: %s, attempts: %d, executions: %d]\n",
			testutil.GetType(fb), fb, e.Result, e.Error, e.Attempts(), e.Executions())
	})
	return fb
}

func withStatsAndLogs[P any, R any](policy failsafe.FailurePolicyBuilder[P, R], stats *Stats, withLogging bool) {
	policy.OnSuccess(func(e failsafe.ExecutionEvent[R]) {
		stats.executionCount.Add(1)
		stats.successCount.Add(1)
		if withLogging {
			fmt.Printf("%s %p success [result: %v, attempts: %d, executions: %d]\n",
				testutil.GetType(policy), policy, e.LastResult(), e.Attempts(), e.Executions())
		}
	})
	policy.OnFailure(func(e failsafe.ExecutionEvent[R]) {
		stats.executionCount.Add(1)
		stats.failureCount.Add(1)
		if withLogging {
			fmt.Printf("%s %p failure [result: %v, error: %s, attempts: %d, executions: %d]\n",
				testutil.GetType(policy), policy, e.LastResult(), e.LastError(), e.Attempts(), e.Executions())
		}
	})
}

type Stats struct {
	executionCount atomic.Int32
	successCount   atomic.Int32
	failureCount   atomic.Int32

	// Retry specific stats
	retryCount           atomic.Int32
	retriesExceededCount atomic.Int32
	abortCount           atomic.Int32
}

func (s *Stats) Executions() int {
	return int(s.executionCount.Load())
}

func (s *Stats) Successes() int {
	return int(s.successCount.Load())
}

func (s *Stats) Failures() int {
	return int(s.failureCount.Load())
}

func (s *Stats) Retries() int {
	return int(s.retryCount.Load())
}

func (s *Stats) RetriesExceeded() int {
	return int(s.retriesExceededCount.Load())
}

func (s *Stats) Aborts() int {
	return int(s.abortCount.Load())
}

func (s *Stats) Reset() {
	s.executionCount.Store(0)
	s.successCount.Store(0)
	s.failureCount.Store(0)

	// Retry specific stats
	s.retryCount.Store(0)
	s.retriesExceededCount.Store(0)
	s.abortCount.Store(0)
}
