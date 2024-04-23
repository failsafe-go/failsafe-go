package ratelimiter

import (
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go/internal/util"
)

type rateLimiterStats interface {
	// acquirePermits eagerly acquires requestedPermits and returns the time that must be waited in order to use the permits,
	// else returns -1 if the wait time would exceed the maxWaitTime. A maxWaitTime of -1 indicates no max wait.
	acquirePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration

	reset()
}

// A rate limiter implementation that evenly distributes permits over time, based on the max permits per period. This
// implementation focuses on the interval between permits, and tracks the next interval in which a permit is free.
type smoothRateLimiterStats[R any] struct {
	config    *rateLimiterConfig[R]
	stopwatch util.Stopwatch
	mtx       sync.Mutex

	// The amount of time, relative to the start time, that the next permit will be free.
	// Will be a multiple of the config.interval.
	// Guarded by mtx
	nextFreePermitTime time.Duration
}

func (s *smoothRateLimiterStats[R]) acquirePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	currentTime := s.stopwatch.ElapsedTime()
	requestedPermitTime := s.config.interval * time.Duration(requestedPermits)
	var waitTime time.Duration
	var newNextFreePermitTime time.Duration

	// If a permit is currently available
	if currentTime >= s.nextFreePermitTime {
		// Time at the start of the current interval
		currentIntervalTime := util.RoundDown(currentTime, s.config.interval)
		newNextFreePermitTime = currentIntervalTime + requestedPermitTime
	} else {
		newNextFreePermitTime = s.nextFreePermitTime + requestedPermitTime
	}

	waitTime = max(newNextFreePermitTime-currentTime-s.config.interval, time.Duration(0))
	if exceedsMaxWaitTime(waitTime, maxWaitTime) {
		return -1
	}

	s.nextFreePermitTime = newNextFreePermitTime
	return waitTime
}

func (s *smoothRateLimiterStats[R]) reset() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.stopwatch.Reset()
	s.nextFreePermitTime = 0
}

// A rate limiter implementation that allows bursts of executions, up to the max permits per period. This implementation
// tracks the current period and available permits, which can go into a deficit. A deficit of available permits will
// cause wait times for callers that can be several periods long, depending on the size of the deficit and the number of
// requested permits.
type burstyRateLimiterStats[R any] struct {
	config    *rateLimiterConfig[R]
	stopwatch util.Stopwatch
	mtx       sync.Mutex

	// Available permits. Can be negative during a deficit.
	// Guarded by mtx
	availablePermits int
	currentPeriod    int
}

func (s *burstyRateLimiterStats[R]) acquirePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	currentTime := s.stopwatch.ElapsedTime()
	newCurrentPeriod := int(currentTime / s.config.period)

	// Update current period and available permits
	if s.currentPeriod < newCurrentPeriod {
		elapsedPeriods := newCurrentPeriod - s.currentPeriod
		elapsedPermits := elapsedPeriods * s.config.periodPermits
		s.currentPeriod = newCurrentPeriod
		if s.availablePermits < 0 {
			s.availablePermits += elapsedPermits
		} else {
			s.availablePermits = s.config.periodPermits
		}
	}

	waitTime := 0 * time.Second
	if requestedPermits > s.availablePermits {
		nextPeriodTime := time.Duration(s.currentPeriod+1) * s.config.period
		timeToNextPeriod := nextPeriodTime - currentTime
		permitDeficit := requestedPermits - s.availablePermits
		additionalPeriods := permitDeficit / s.config.periodPermits
		additionalUnits := permitDeficit % s.config.periodPermits

		// Do not wait for an additional period if we're not using any permits from it
		if additionalUnits == 0 {
			additionalPeriods -= 1
		}

		// The time to wait until the beginning of the next period that will have free permits
		waitTime = timeToNextPeriod + (time.Duration(additionalPeriods) * s.config.period)
		if exceedsMaxWaitTime(waitTime, maxWaitTime) {
			return -1
		}
	}

	s.availablePermits -= requestedPermits
	return waitTime
}

func (s *burstyRateLimiterStats[R]) reset() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.stopwatch.Reset()
	s.availablePermits = s.config.periodPermits
	s.currentPeriod = 0
}

// exceedsMaxWaitTime returns whether the waitTime would exceed the maxWaitTime, else false if maxWaitTime is -1.
func exceedsMaxWaitTime(waitTime time.Duration, maxWaitTime time.Duration) bool {
	return maxWaitTime != -1 && waitTime > maxWaitTime
}
