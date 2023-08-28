package circuitbreaker

import (
	"time"

	"failsafe"
	"failsafe/internal/util"
)

type circuitState[R any] interface {
	getState() State
	getStats() circuitStats
	getRemainingDelay() time.Duration
	tryAcquirePermit() bool
	checkThresholdAndReleasePermit(exec *failsafe.Execution[R])
}

type closedState[R any] struct {
	breaker *circuitBreaker[R]
	stats   circuitStats
}

var _ circuitState[any] = &closedState[any]{}

func newClosedState[R any](breaker *circuitBreaker[R]) *closedState[R] {
	var capacity uint
	if breaker.config.failureThresholdConfig.executionThreshold != 0 {
		capacity = breaker.config.failureThresholdConfig.executionThreshold
	} else {
		capacity = breaker.config.failureThresholdConfig.thresholdingCapacity
	}
	return &closedState[R]{
		breaker: breaker,
		stats:   newStats(breaker.config, true, capacity),
	}
}

func (s *closedState[R]) getState() State {
	return ClosedState
}

func (s *closedState[R]) getStats() circuitStats {
	return s.stats
}

func (s *closedState[R]) getRemainingDelay() time.Duration {
	return 0
}

func (s *closedState[R]) tryAcquirePermit() bool {
	return true
}

func (s *closedState[R]) checkThresholdAndReleasePermit(exec *failsafe.Execution[R]) {
	// Execution threshold can only be set for time based thresholding
	if s.stats.getExecutionCount() >= s.breaker.config.failureThresholdConfig.executionThreshold {
		// Failure rate threshold can only be set for time based thresholding
		failureRateThreshold := s.breaker.config.failureThresholdConfig.rateThreshold
		if (failureRateThreshold != 0 && s.stats.getFailureRate() >= failureRateThreshold) ||
			(failureRateThreshold == 0 && s.stats.getFailureCount() >= s.breaker.config.failureThresholdConfig.threshold) {
			s.breaker.open(exec)
		}
	}
}

type openState[R any] struct {
	breaker   *circuitBreaker[R]
	stats     circuitStats
	startTime int64
	delay     time.Duration
}

var _ circuitState[any] = &openState[any]{}

func newOpenState[R any](breaker *circuitBreaker[R], previousState circuitState[R], delay time.Duration) *openState[R] {
	return &openState[R]{
		breaker:   breaker,
		stats:     previousState.getStats(),
		startTime: breaker.config.clock.CurrentUnixNano(),
		delay:     delay,
	}
}

func (s *openState[R]) getState() State {
	return OpenState
}

func (s *openState[R]) getStats() circuitStats {
	return s.stats
}

func (s *openState[R]) getRemainingDelay() time.Duration {
	elapsedTime := s.breaker.config.clock.CurrentUnixNano() - s.startTime
	return util.Max(0, s.delay-time.Duration(elapsedTime))
}

func (s *openState[R]) tryAcquirePermit() bool {
	if s.breaker.config.clock.CurrentUnixNano()-s.startTime >= s.delay.Nanoseconds() {
		s.breaker.halfOpen()
		return s.breaker.TryAcquirePermit()
	}
	return false
}

func (s *openState[R]) checkThresholdAndReleasePermit(_ *failsafe.Execution[R]) {
}

type halfOpenState[R any] struct {
	breaker             *circuitBreaker[R]
	stats               circuitStats
	permittedExecutions int
}

var _ circuitState[any] = &halfOpenState[any]{}

func newHalfOpenState[R any](breaker *circuitBreaker[R]) *halfOpenState[R] {
	capacity := breaker.config.successThresholdingCapacity
	if capacity == 0 {
		capacity = breaker.config.failureThresholdConfig.executionThreshold
	}
	if capacity == 0 {
		capacity = breaker.config.failureThresholdConfig.thresholdingCapacity
	}
	return &halfOpenState[R]{
		breaker: breaker,
		stats:   newStats[R](breaker.config, false, capacity),
	}
}

func (s *halfOpenState[R]) getState() State {
	return HalfOpenState
}

func (s *halfOpenState[R]) getStats() circuitStats {
	return s.stats
}

func (s *halfOpenState[R]) getRemainingDelay() time.Duration {
	return 0
}

func (s *halfOpenState[R]) tryAcquirePermit() bool {
	result := s.permittedExecutions > 0
	s.permittedExecutions--
	return result
}

/*
*
Checks to determine if a threshold has been met and the circuit should be opened or closed.
If a success threshold is configured, the circuit is opened or closed based on whether the ratio was exceeded.
Else the circuit is opened or closed based on whether the failure threshold was exceeded.
A permit is released before returning.
*/
func (s *halfOpenState[R]) checkThresholdAndReleasePermit(exec *failsafe.Execution[R]) {
	var successesExceeded bool
	var failuresExceeded bool

	successThreshold := s.breaker.config.successThreshold
	if successThreshold != 0 {
		successThresholdingCapacity := s.breaker.config.successThresholdingCapacity
		successesExceeded = s.stats.getSuccessCount() >= successThreshold
		failuresExceeded = s.stats.getFailureCount() > successThresholdingCapacity-successThreshold
	} else {
		// Failure rate threshold can only be set for time based thresholding
		failureRateThreshold := s.breaker.config.failureThresholdConfig.rateThreshold
		if failureRateThreshold != 0 {
			// Execution threshold can only be set for time based thresholding
			executionThresholdExceeded := s.stats.getExecutionCount() >= s.breaker.config.failureThresholdConfig.executionThreshold
			failuresExceeded = executionThresholdExceeded && s.stats.getFailureRate() >= failureRateThreshold
			successesExceeded = executionThresholdExceeded && s.stats.getSuccessRate() > 100-failureRateThreshold
		} else {
			failureThresholdingCapacity := s.breaker.config.failureThresholdConfig.thresholdingCapacity
			failureThreshold := s.breaker.config.failureThresholdConfig.threshold
			failuresExceeded = s.stats.getFailureCount() >= failureThreshold
			successesExceeded = s.stats.getSuccessCount() > failureThresholdingCapacity-failureThreshold
		}
	}

	if successesExceeded {
		s.breaker.close()
	} else if failuresExceeded {
		s.breaker.open(exec)
	}
	s.permittedExecutions++
}
