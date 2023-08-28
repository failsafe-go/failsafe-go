package ratelimiter

import (
	"time"

	"failsafe/internal/util"
)

type rateLimiterStats interface {
	// acquirePermits eagerly acquires requestedPermits and returns the time that must be waited in order to use the permits, else
	// returns -1 if the wait time would exceed the maxWaitTime. A maxWaitTime of -1 indicates no max wait.
	acquirePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration
}
type smoothRateLimiterStats[R any] struct {
	config    *rateLimiterConfig[R]
	stopwatch util.Stopwatch

	// The amount of time, relative to the start time, that the next permit will be free.
	// Will be a multiple of the config.interval.
	nextFreePermitTime time.Duration
}

type burstyRateLimiterStats[R any] struct {
	config *rateLimiterConfig[R]
	util.Stopwatch

	// Available permits. Can be negative during a deficit.
	availablePermits int
	currentPeriod    int
}

func (s *smoothRateLimiterStats[R]) acquirePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration {
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

	waitTime = util.Max(newNextFreePermitTime-currentTime-s.config.interval, time.Duration(0))
	if exceedsMaxWaitTime(waitTime, maxWaitTime) {
		return -1
	}

	s.nextFreePermitTime = newNextFreePermitTime
	return waitTime
}

func (s *burstyRateLimiterStats[R]) acquirePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration {
	currentTime := s.Stopwatch.ElapsedTime()
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

		waitTime = timeToNextPeriod + (time.Duration(additionalPeriods) * s.config.period)
		if exceedsMaxWaitTime(waitTime, maxWaitTime) {
			return -1
		}
	}

	s.availablePermits -= requestedPermits
	return waitTime
}

func exceedsMaxWaitTime(waitTime time.Duration, maxWaitTime time.Duration) bool {
	if maxWaitTime != -1 && waitTime > maxWaitTime {
		return true
	}
	return false
}
