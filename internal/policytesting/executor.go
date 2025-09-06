// Package policytesting is needed to avoid a circular dependency with the policy package.
package policytesting

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/adaptivelimiter"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/cachepolicy"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

func Reset[R any](p failsafe.Policy[R]) {
	elem := reflect.ValueOf(p)
	resetMethod := elem.MethodByName("Reset")
	if !resetMethod.IsValid() {
		panic("Failed to reflect Reset method")
	}
	resetMethod.Call([]reflect.Value{})
}

func WithRetryStats[R any](rp retrypolicy.Builder[R], stats *Stats) retrypolicy.Builder[R] {
	return withRetryStatsAndLogs(rp, stats, false)
}

func WithRetryLogs[R any](rp retrypolicy.Builder[R]) retrypolicy.Builder[R] {
	return withRetryStatsAndLogs(rp, &Stats{}, true)
}

func WithRetryStatsAndLogs[R any](rp retrypolicy.Builder[R], stats *Stats) retrypolicy.Builder[R] {
	return withRetryStatsAndLogs(rp, stats, true)
}

func withRetryStatsAndLogs[R any](rp retrypolicy.Builder[R], stats *Stats, withLogging bool) retrypolicy.Builder[R] {
	rp.OnRetry(func(e failsafe.ExecutionEvent[R]) {
		stats.retries.Add(1)
		if withLogging {
			fmt.Printf("retrypolicy %p retrying [result: %v, error: %s]\n", rp, e.LastResult(), e.LastError())
		}
	}).OnRetriesExceeded(func(e failsafe.ExecutionEvent[R]) {
		stats.retriesExceeded.Add(1)
		if withLogging {
			fmt.Printf("retrypolicy %p exceeded\n", rp)
		}
	}).OnAbort(func(e failsafe.ExecutionEvent[R]) {
		stats.aborts.Add(1)
		if withLogging {
			fmt.Printf("retrypolicy %p aborted\n", rp)
		}
	})
	withStatsAndLogs[retrypolicy.Builder[R], R](rp, stats, withLogging)
	return rp
}

func WithAdaptiveLimiterStatsAndLogs[R any](l adaptivelimiter.Builder[R], stats *Stats, withLogging bool) adaptivelimiter.Builder[R] {
	l.OnLimitChanged(func(event adaptivelimiter.LimitChangedEvent) {
		if withLogging {
			stats.limitsChanged.Add(1)
			fmt.Printf("adaptivelimiter %p changed\n", l)
		}
	}).OnLimitExceeded(func(e failsafe.ExecutionEvent[R]) {
		if withLogging {
			stats.limitsExceeded.Add(1)
			fmt.Printf("adaptivelimiter %p exceeded\n", l)
		}
	})
	return l
}

func WithBreakerStats[R any](cb circuitbreaker.Builder[R], stats *Stats) circuitbreaker.Builder[R] {
	withBreakerStatsAndLogs(cb, stats, false)
	return cb
}

func WithBreakerLogs[R any](cb circuitbreaker.Builder[R]) circuitbreaker.Builder[R] {
	withBreakerStatsAndLogs(cb, &Stats{}, true)
	return cb
}

func withBreakerStatsAndLogs[R any](cb circuitbreaker.Builder[R], stats *Stats, withLogging bool) circuitbreaker.Builder[R] {
	cb.OnOpen(func(event circuitbreaker.StateChangedEvent) {
		if withLogging {
			fmt.Printf("circuitbreaker %p opened\n", cb)
		}
	})
	cb.OnHalfOpen(func(event circuitbreaker.StateChangedEvent) {
		if withLogging {
			fmt.Printf("circuitbreaker %p half-opened\n", cb)
		}
	})
	cb.OnClose(func(event circuitbreaker.StateChangedEvent) {
		if withLogging {
			fmt.Printf("circuitbreaker %p closed\n", cb)
		}
	})
	withStatsAndLogs[circuitbreaker.Builder[R], R](cb, stats, withLogging)
	return cb
}

func WithTimeoutStatsAndLogs[R any](to timeout.Builder[R], stats *Stats) timeout.Builder[R] {
	to.OnTimeoutExceeded(func(e failsafe.ExecutionDoneEvent[R]) {
		stats.executions.Add(1)
		fmt.Printf("timeout %p exceeded [attempts: %d, executions: %d]\n", to, e.Attempts(), e.Executions())
	})
	return to
}

func WithFallbackStatsAndLogs[R any](fb fallback.Builder[R], stats *Stats) fallback.Builder[R] {
	fb.OnFallbackExecuted(func(e failsafe.ExecutionDoneEvent[R]) {
		stats.executions.Add(1)
		fmt.Printf("fallback %p executed [result: %v, error: %s, attempts: %d, executions: %d]\n",
			fb, e.Result, e.Error, e.Attempts(), e.Executions())
	})
	return fb
}

func WithHedgeStatsAndLogs[R any](hp hedgepolicy.Builder[R], stats *Stats) hedgepolicy.Builder[R] {
	hp.OnHedge(func(e failsafe.ExecutionEvent[R]) {
		stats.hedges.Add(1)
		fmt.Printf("hedge %p starting [attempts: %v]\n", hp, e.Attempts())
	})
	return hp
}

func WithBulkheadStatsAndLogs[R any](bh bulkhead.Builder[R], stats *Stats, withLogging bool) bulkhead.Builder[R] {
	bh.OnFull(func(event failsafe.ExecutionEvent[R]) {
		if withLogging {
			stats.fulls.Add(1)
			fmt.Printf("bulkhead %p full\n", bh)
		}
	})
	return bh
}

func WithCacheStats[R any](cp cachepolicy.Builder[R], stats *Stats) cachepolicy.Builder[R] {
	cp.OnCacheHit(func(e failsafe.ExecutionDoneEvent[R]) {
		stats.cacheHits.Add(1)
	}).OnCacheMiss(func(e failsafe.ExecutionEvent[R]) {
		stats.cacheMisses.Add(1)
	}).OnResultCached(func(event failsafe.ExecutionEvent[R]) {
		stats.caches.Add(1)
	})
	return cp
}

func withStatsAndLogs[P any, R any](policy failsafe.FailurePolicyBuilder[P, R], stats *Stats, withLogging bool) {
	policy.OnSuccess(func(e failsafe.ExecutionEvent[R]) {
		stats.executions.Add(1)
		stats.successes.Add(1)
		if withLogging {
			fmt.Printf("policy %p success [result: %v, attempts: %d, executions: %d]\n",
				policy, e.LastResult(), e.Attempts(), e.Executions())
		}
	})
	policy.OnFailure(func(e failsafe.ExecutionEvent[R]) {
		stats.executions.Add(1)
		stats.failures.Add(1)
		if withLogging {
			fmt.Printf("policy %p failure [result: %v, error: %s, attempts: %d, executions: %d]\n",
				policy, e.LastResult(), e.LastError(), e.Attempts(), e.Executions())
		}
	})
}

type Stats struct {
	executions atomic.Int32
	successes  atomic.Int32
	failures   atomic.Int32

	// Retry specific stats
	retries         atomic.Int32
	retriesExceeded atomic.Int32
	aborts          atomic.Int32

	limitsExceeded atomic.Int32
	limitsChanged  atomic.Int32

	// Hedge specific stats
	hedges atomic.Int32

	// Bulkhead specific stats
	fulls atomic.Int32

	// Cache specific stats
	caches      atomic.Int32
	cacheHits   atomic.Int32
	cacheMisses atomic.Int32
	cachedCount atomic.Int32
}

func (s *Stats) Executions() int {
	return int(s.executions.Load())
}

func (s *Stats) Successes() int {
	return int(s.successes.Load())
}

func (s *Stats) Failures() int {
	return int(s.failures.Load())
}

func (s *Stats) Retries() int {
	return int(s.retries.Load())
}

func (s *Stats) RetriesExceeded() int {
	return int(s.retriesExceeded.Load())
}

func (s *Stats) LimitsChanged() int {
	return int(s.limitsChanged.Load())
}

func (s *Stats) LimitsExceeded() int {
	return int(s.limitsExceeded.Load())
}

func (s *Stats) Hedges() int {
	return int(s.hedges.Load())
}

func (s *Stats) Aborts() int {
	return int(s.aborts.Load())
}

func (s *Stats) Fulls() int {
	return int(s.fulls.Load())
}

func (s *Stats) CacheHits() int {
	return int(s.cacheHits.Load())
}

func (s *Stats) CacheMisses() int {
	return int(s.cacheMisses.Load())
}

func (s *Stats) Caches() int {
	return int(s.caches.Load())
}

func (s *Stats) Reset() {
	s.executions.Store(0)
	s.successes.Store(0)
	s.failures.Store(0)

	// Retry specific stats
	s.retries.Store(0)
	s.retriesExceeded.Store(0)
	s.aborts.Store(0)

	// AdaptiveLimiter specific stats
	s.limitsExceeded.Store(0)
	s.limitsChanged.Store(0)

	// Hedge specific stats
	s.hedges.Store(0)

	// Bulkhead specific stats
	s.fulls.Store(0)

	// Cache specific stats
	s.caches.Store(0)
	s.cacheHits.Store(0)
	s.cacheMisses.Store(0)
}

func SetupFn(stats *Stats) func() context.Context {
	return func() context.Context {
		stats.Reset()
		return nil
	}
}
