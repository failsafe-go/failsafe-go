package adaptivelimiter

import (
	"context"
	"math/rand"
	"time"

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

// levelRange provides a wider range of levels that allow for rejecting a subset of executions within a Priority.
type levelRange struct {
	lower, upper int
}

var priorityLevelRanges = map[Priority]levelRange{
	PriorityVeryLow:  {0, 99},
	PriorityLow:      {100, 199},
	PriorityMedium:   {200, 299},
	PriorityHigh:     {300, 399},
	PriorityVeryHigh: {400, 499},
}

type key int

// PriorityKey is a key to use with a Context that stores the priority value.
const PriorityKey key = 0

// LevelKey is a key to use with a Context that stores the level value.
const LevelKey key = 1

// ContextWithPriority returns a context with the priority value.
func ContextWithPriority(ctx context.Context, priority Priority) context.Context {
	return context.WithValue(ctx, PriorityKey, priority)
}

// ContextWithLevel returns a context with the level value.
func ContextWithLevel(ctx context.Context, level int) context.Context {
	return context.WithValue(ctx, LevelKey, level)
}

// PriorityLimiter is an adaptive concurrency limiter that can prioritize execution rejections during overload. When the
// limiter and its queue start to become full, it uses a Prioritizer to determine which priority levels should be
// rejected, allowing higher-priority executions to proceed while shedding lower-priority load.
//
// R is the execution result type. This type is concurrency safe.
// PriorityLimiter is an adaptive concurrency limiter that can prioritize execution rejections via a Prioritizer.
//
// R is the execution result type. This type is concurrency safe.
type PriorityLimiter[R any] interface {
	failsafe.Policy[R]
	Metrics

	// AcquirePermit attempts to acquire a permit for an execution at the priority or level contained in the context,
	// waiting until one is available or the execution is canceled. Returns [context.Canceled] if the ctx is canceled. A
	// priority must be stored in the context using the PriorityKey, and a level must be stored in the context using the
	// LevelKey. The execution's priority must be greater than the current rejection threshold for admission. Levels must be
	// between 0 and 499.
	//
	// Example usage:
	//   ctx := context.WithValue(ctx, PriorityKey, PriorityHigh)
	//   permit, err := limiter.AcquirePermit(ctx)
	AcquirePermit(ctx context.Context) (Permit, error)

	// AcquirePermitWithMaxWait attempts to acquire a permit for an execution at the priority or level contained in the
	// context, waiting until one is available, the execution is canceled, or the maxWaitTime is exceeded. Returns
	// [context.Canceled] if the ctx is canceled. A priority must be stored in the context using the PriorityKey, and a
	// level must be stored in the context using the LevelKey. The execution's priority must be greater than the current
	// rejection threshold for admission. Levels must be between 0 and 499.
	AcquirePermitWithMaxWait(ctx context.Context, maxWaitTime time.Duration) (Permit, error)

	// AcquirePermitWithPriority attempts to acquire a permit for a execution at the given priority, waiting until one is
	// available or the execution is canceled. Returns [context.Canceled] if the ctx is canceled. The execution priority must
	// be greater than the current rejection threshold for admission.
	AcquirePermitWithPriority(ctx context.Context, priority Priority) (Permit, error)

	// AcquirePermitWithLevel attempts to acquire a permit for a execution at the given priority level, waiting until one is
	// available or the execution is canceled. Returns [context.Canceled] if the ctx is canceled. The execution priority level
	// must be greater than the current rejection threshold for admission, and the level must be between 0 and 499.
	AcquirePermitWithLevel(ctx context.Context, level int) (Permit, error)

	// CanAcquirePermit returns whether it's currently possible to acquire a permit for the priority or level contained in
	// the context. If a priority and level are both provided, the level takes precedent. If no priority or level are
	// provided, level 0 is used.
	CanAcquirePermit(ctx context.Context) bool

	// CanAcquirePermitWithPriority returns whether it's currently possible to acquire a permit for the priority.
	CanAcquirePermitWithPriority(priority Priority) bool

	// CanAcquirePermitWithLevel returns whether it's currently possible to acquire a permit for the level. The level must
	// be between 0 and 499.
	CanAcquirePermitWithLevel(level int) bool

	// Reset resets the limiter to its initial limit.
	Reset()
}

type priorityLimiter[R any] struct {
	*queueingLimiter[R]
	prioritizer Prioritizer
}

func (l *priorityLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
	return l.AcquirePermitWithLevel(ctx, levelForContext(ctx))
}

func (l *priorityLimiter[R]) AcquirePermitWithMaxWait(ctx context.Context, maxWaitTime time.Duration) (Permit, error) {
	level := levelForContext(ctx)
	if !l.CanAcquirePermitWithLevel(level) {
		return nil, ErrExceeded
	}

	permit, err := l.adaptiveLimiter.AcquirePermitWithMaxWait(ctx, maxWaitTime)
	if err != nil {
		return nil, err
	}
	l.prioritizer.recordPriority(level)
	return permit, nil
}

func (l *priorityLimiter[R]) AcquirePermitWithPriority(ctx context.Context, priority Priority) (Permit, error) {
	return l.AcquirePermitWithLevel(ctx, randomLevel(priority))
}

func (l *priorityLimiter[R]) AcquirePermitWithLevel(ctx context.Context, level int) (Permit, error) {
	if !l.CanAcquirePermitWithLevel(level) {
		return nil, ErrExceeded
	}

	permit, err := l.adaptiveLimiter.AcquirePermit(ctx)
	if err != nil {
		return nil, err
	}
	l.prioritizer.recordPriority(level)
	return permit, nil
}

func (l *priorityLimiter[R]) CanAcquirePermit(ctx context.Context) bool {
	return l.CanAcquirePermitWithLevel(levelForContext(ctx))
}

func (l *priorityLimiter[R]) CanAcquirePermitWithPriority(priority Priority) bool {
	return l.CanAcquirePermitWithLevel(randomLevel(priority))
}

func (l *priorityLimiter[R]) CanAcquirePermitWithLevel(level int) bool {
	// Return immediately if the limiter has capacity
	if l.adaptiveLimiter.CanAcquirePermit() {
		return true
	}

	// Check the limiter's max capacity
	_, _, _, maxQueue := l.queueStats()
	if l.Queued() >= maxQueue {
		return false
	}
	context.WithValue(context.Background(), PriorityKey, PriorityHigh)
	// Threshold against the prioritizer's rejection threshold
	return level >= l.prioritizer.RejectionThreshold()
}

func randomLevel(priority Priority) int {
	r := priorityLevelRanges[priority]
	return rand.Intn(r.upper-r.lower+1) + r.lower
}

func levelForContext(ctx context.Context) int {
	var level int
	if untypedLevel := ctx.Value(LevelKey); untypedLevel != nil {
		level, _ = untypedLevel.(int)
	}
	if level == 0 {
		if untypedPriority := ctx.Value(PriorityKey); untypedPriority != nil {
			priority, _ := untypedPriority.(Priority)
			// Generate a random level if we only have a priority
			level = randomLevel(priority)
		}
	}
	return level
}

func (l *priorityLimiter[R]) ToExecutor(_ R) any {
	e := &executor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		blockingLimiter: l,
	}
	e.Executor = e
	return e
}

func (l *priorityLimiter[R]) configRef() *config[R] {
	return l.config
}
