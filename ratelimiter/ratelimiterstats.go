package ratelimiter

import (
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go/internal/util"
)

type stats interface {
	// acquirePermits eagerly acquires requestedPermits and returns the time that must be waited in order to use the permits,
	// else returns -1 if the wait time would exceed the maxWaitTime. A maxWaitTime of -1 indicates no max wait.
	acquirePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration

	reset()
}

// A rate limiter implementation that evenly distributes permits over time, based on the max permits per period. This
// implementation focuses on the interval between permits, and tracks the next interval in which a permit is free.
type smoothStats[R any] struct {
	*config[R]
	stopwatch util.Stopwatch
	mu        sync.Mutex

	// The amount of time, relative to the start time, that the next permit will be free.
	// Will be a multiple of the config.interval.
	// Guarded by mu
	nextFreePermitTime time.Duration
}

func (s *smoothStats[R]) acquirePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentTime := s.stopwatch.ElapsedTime()
	requestedPermitTime := s.interval * time.Duration(requestedPermits)
	var newNextFreePermitTime time.Duration

	// If a permit is currently available
	if currentTime >= s.nextFreePermitTime {
		// Time at the start of the current interval
		currentIntervalTime := util.RoundDown(currentTime, s.interval)
		newNextFreePermitTime = currentIntervalTime + requestedPermitTime
	} else {
		newNextFreePermitTime = s.nextFreePermitTime + requestedPermitTime
	}

	waitTime := max(newNextFreePermitTime-currentTime-s.interval, time.Duration(0))
	if exceedsMaxWaitTime(waitTime, maxWaitTime) {
		return -1
	}

	s.nextFreePermitTime = newNextFreePermitTime
	return waitTime
}

func (s *smoothStats[R]) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopwatch.Reset()
	s.nextFreePermitTime = 0
}

// A rate limiter implementation that allows bursts of executions, up to the max permits per period. This implementation
// tracks the current period and available permits, which can go into a deficit. A deficit of available permits will
// cause wait times for callers that can be several periods long, depending on the size of the deficit and the number of
// requested permits.
type burstyStats[R any] struct {
	*config[R]
	stopwatch util.Stopwatch
	mu        sync.Mutex

	// Available permits. Can be negative during a deficit.
	// Guarded by mu
	availablePermits int
	currentPeriod    int
}

func (s *burstyStats[R]) acquirePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentTime := s.stopwatch.ElapsedTime()
	newCurrentPeriod := int(currentTime / s.period)

	// Update current period and available permits
	if s.currentPeriod < newCurrentPeriod {
		elapsedPeriods := newCurrentPeriod - s.currentPeriod
		elapsedPermits := elapsedPeriods * s.periodPermits
		s.currentPeriod = newCurrentPeriod
		if s.availablePermits < 0 {
			s.availablePermits += elapsedPermits
		} else {
			s.availablePermits = s.periodPermits
		}
	}

	waitTime := 0 * time.Second
	if requestedPermits > s.availablePermits {
		nextPeriodTime := time.Duration(s.currentPeriod+1) * s.period
		timeToNextPeriod := nextPeriodTime - currentTime
		permitDeficit := requestedPermits - s.availablePermits
		additionalPeriods := permitDeficit / s.periodPermits
		additionalUnits := permitDeficit % s.periodPermits

		// Do not wait for an additional period if we're not using any permits from it
		if additionalUnits == 0 {
			additionalPeriods -= 1
		}

		// The time to wait until the beginning of the next period that will have free permits
		waitTime = timeToNextPeriod + (time.Duration(additionalPeriods) * s.period)
		if exceedsMaxWaitTime(waitTime, maxWaitTime) {
			return -1
		}
	}

	s.availablePermits -= requestedPermits
	return waitTime
}

func (s *burstyStats[R]) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopwatch.Reset()
	s.availablePermits = s.periodPermits
	s.currentPeriod = 0
}

// exceedsMaxWaitTime returns whether the waitTime would exceed the maxWaitTime, else false if maxWaitTime is -1.
func exceedsMaxWaitTime(waitTime time.Duration, maxWaitTime time.Duration) bool {
	return maxWaitTime != -1 && waitTime > maxWaitTime
}
