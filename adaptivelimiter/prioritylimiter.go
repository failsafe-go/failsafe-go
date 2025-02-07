package adaptivelimiter

import (
	"context"
	"math/rand"
	"sync/atomic"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

type Priority int

const (
	PriorityVeryLow Priority = iota
	PriorityLow
	PriorityMedium
	PriorityHigh
	PriorityVeryHigh
)

// priorityRange provides a wider range of priorities that allow for rejecting a subset of requests within a Priority.
type priorityRange struct {
	lower, upper int
}

// Defining the priority ranges as a map
var priorityRanges = map[Priority]priorityRange{
	PriorityVeryLow:  {0, 99},
	PriorityLow:      {100, 199},
	PriorityMedium:   {200, 299},
	PriorityHigh:     {300, 399},
	PriorityVeryHigh: {400, 499},
}

func randomGranularPriority(priority Priority) int {
	r := priorityRanges[priority]
	return rand.Intn(r.upper-r.lower+1) + r.lower
}

type key int

// PriorityKey is a key to use with a Context that stores the priority value.
const PriorityKey key = 0

// PriorityLimiter is an adaptive concurrency limiter that can prioritize request rejections via a Prioritizer.
type PriorityLimiter[R any] interface {
	failsafe.Policy[R]
	Metrics

	// AcquirePermit attempts to acquire a permit, potentially blocking up to maxExecutionTime.
	// The request priority must be greater than the current priority threshold for admission.
	AcquirePermit(ctx context.Context, priority Priority) (Permit, error)

	// CanAcquirePermit returns whether it's currently possible to acquire a permit for the priority.
	CanAcquirePermit(priority Priority) bool
}

type priorityLimiter[R any] struct {
	*adaptiveLimiter[R]

	inCount  atomic.Uint32 // Requests received in current calibration period
	outCount atomic.Uint32 // Requests permitted in current calibration period
}

func (l *priorityLimiter[R]) AcquirePermit(ctx context.Context, priority Priority) (Permit, error) {
	// Generate a granular priority for the request and compare it to the prioritizer threshold
	granularPriority := randomGranularPriority(priority)
	if granularPriority < l.prioritizer.RejectionThreshold() {
		return nil, ErrExceeded
	}

	// Maintain queue stats
	l.inCount.Add(1)
	defer l.outCount.Add(1)

	l.prioritizer.recordPriority(granularPriority)
	return l.adaptiveLimiter.AcquirePermit(ctx)
}

func (l *priorityLimiter[R]) CanAcquirePermit(priority Priority) bool {
	return randomGranularPriority(priority) >= l.prioritizer.RejectionThreshold()
}

func (l *priorityLimiter[R]) RejectionRate() float64 {
	return l.prioritizer.RejectionRate()
}

func (l *priorityLimiter[R]) getAndResetStats() (in, out, limit, inflight, queued, rejectionThreshold, maxQueue int) {
	in = int(l.inCount.Swap(0))
	out = int(l.outCount.Swap(0))
	limit = l.Limit()
	rejectionThreshold = int(float64(limit) * l.initialRejectionFactor)
	maxQueue = int(float64(limit) * l.maxRejectionFactor)
	return in, out, limit, l.Inflight(), l.Blocked(), rejectionThreshold, maxQueue
}

func (l *priorityLimiter[R]) ToExecutor(_ R) any {
	e := &priorityLimiterExecutor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		priorityLimiter: l,
	}
	e.Executor = e
	return e
}
