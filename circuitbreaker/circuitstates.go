package circuitbreaker

import (
	"time"

	"github.com/failsafe-go/failsafe-go"
)

// State of a CircuitBreaker.
// Implementations are not concurrency safe and must be guarded externally.
type circuitState[R any] interface {
	getState() State
	getStats() stats
	getRemainingDelay() time.Duration
	tryAcquirePermit() bool
	checkThresholdAndReleasePermit(exec failsafe.Execution[R])
}

type closedState[R any] struct {
	breaker *circuitBreaker[R]
	stats   stats
}

func newClosedState[R any](breaker *circuitBreaker[R]) *closedState[R] {
	var capacity uint
	if breaker.failureExecutionThreshold != 0 {
		capacity = breaker.failureExecutionThreshold
	} else {
		capacity = breaker.failureThresholdingCapacity
	}
	return &closedState[R]{
		breaker: breaker,
		stats:   newStats(breaker.config, true, capacity),
	}
}

func (s *closedState[R]) getState() State {
	return ClosedState
}

func (s *closedState[R]) getStats() stats {
	return s.stats
}

func (s *closedState[R]) getRemainingDelay() time.Duration {
	return 0
}

func (s *closedState[R]) tryAcquirePermit() bool {
	return true
}

// Checks to see if the executions and failure thresholds have been exceeded, opening the circuit if so.
func (s *closedState[R]) checkThresholdAndReleasePermit(exec failsafe.Execution[R]) {
	// Execution threshold can only be set for time based thresholding
	if s.stats.executionCount() >= s.breaker.failureExecutionThreshold {
		// Failure rate threshold can only be set for time based thresholding
		failureRateThreshold := s.breaker.failureRateThreshold
		if (failureRateThreshold != 0 && s.stats.failureRate() >= failureRateThreshold) ||
			(failureRateThreshold == 0 && s.stats.failureCount() >= s.breaker.failureThreshold) {
			s.breaker.open(exec)
		}
	}
}

type openState[R any] struct {
	breaker   *circuitBreaker[R]
	stats     stats
	startTime int64
	delay     time.Duration
}

func newOpenState[R any](breaker *circuitBreaker[R], previousState circuitState[R], delay time.Duration) *openState[R] {
	return &openState[R]{
		breaker:   breaker,
		stats:     previousState.getStats(),
		startTime: breaker.clock.CurrentUnixNano(),
		delay:     delay,
	}
}

func (s *openState[R]) getState() State {
	return OpenState
}

func (s *openState[R]) getStats() stats {
	return s.stats
}

func (s *openState[R]) getRemainingDelay() time.Duration {
	elapsedTime := s.breaker.clock.CurrentUnixNano() - s.startTime
	return max(0, s.delay-time.Duration(elapsedTime))
}

func (s *openState[R]) tryAcquirePermit() bool {
	if s.breaker.clock.CurrentUnixNano()-s.startTime >= s.delay.Nanoseconds() {
		s.breaker.halfOpen()
		return s.breaker.tryAcquirePermit()
	}
	return false
}

func (s *openState[R]) checkThresholdAndReleasePermit(_ failsafe.Execution[R]) {
}

type halfOpenState[R any] struct {
	breaker             *circuitBreaker[R]
	stats               stats
	permittedExecutions uint
}

func newHalfOpenState[R any](breaker *circuitBreaker[R]) *halfOpenState[R] {
	capacity := breaker.successThresholdingCapacity
	if capacity == 0 {
		capacity = breaker.failureExecutionThreshold
	}
	if capacity == 0 {
		capacity = breaker.failureThresholdingCapacity
	}
	return &halfOpenState[R]{
		breaker:             breaker,
		stats:               newStats[R](breaker.config, false, capacity),
		permittedExecutions: capacity,
	}
}

func (s *halfOpenState[R]) getState() State {
	return HalfOpenState
}

func (s *halfOpenState[R]) getStats() stats {
	return s.stats
}

func (s *halfOpenState[R]) getRemainingDelay() time.Duration {
	return 0
}

func (s *halfOpenState[R]) tryAcquirePermit() bool {
	if s.permittedExecutions > 0 {
		s.permittedExecutions--
		return true
	}
	return false
}

/*
Checks to determine if a threshold has been met and the circuit should be opened or closed.
  - If a success threshold is configured, the circuit is opened or closed based on whether the ratio was exceeded.
  - Else the circuit is opened or closed based on whether the failure threshold was exceeded.

A permit is released before returning.
*/
func (s *halfOpenState[R]) checkThresholdAndReleasePermit(exec failsafe.Execution[R]) {
	var successesExceeded bool
	var failuresExceeded bool

	successThreshold := s.breaker.successThreshold
	if successThreshold != 0 {
		successThresholdingCapacity := s.breaker.successThresholdingCapacity
		successesExceeded = s.stats.successCount() >= successThreshold
		failuresExceeded = s.stats.failureCount() > successThresholdingCapacity-successThreshold
	} else {
		// Failure rate threshold can only be set for time based thresholding
		failureRateThreshold := s.breaker.failureRateThreshold
		if failureRateThreshold != 0 {
			// Execution threshold can only be set for time based thresholding
			executionThresholdExceeded := s.stats.executionCount() >= s.breaker.failureExecutionThreshold
			failuresExceeded = executionThresholdExceeded && s.stats.failureRate() >= failureRateThreshold
			successesExceeded = executionThresholdExceeded && s.stats.successRate() > 100-failureRateThreshold
		} else {
			failureThresholdingCapacity := s.breaker.failureThresholdingCapacity
			failureThreshold := s.breaker.failureThreshold
			failuresExceeded = s.stats.failureCount() >= failureThreshold
			successesExceeded = s.stats.successCount() > failureThresholdingCapacity-failureThreshold
		}
	}

	if successesExceeded {
		s.breaker.close()
	} else if failuresExceeded {
		s.breaker.open(exec)
	}
	s.permittedExecutions++
}
