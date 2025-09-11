package adaptivethrottler

import (
	"context"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
	"github.com/failsafe-go/failsafe-go/priority"
)

// PriorityThrottler is an adaptive throttler that throttles load probabalistically based on recent failures. When
// throttling is needed, it uses a Prioritizer to determine which priority levels should be rejected, allowing
// higher-priority executions to proceed while shedding lower-priority load.
//
// R is the execution result type. This type is concurrency safe.
type PriorityThrottler[R any] interface {
	failsafe.Policy[R]
	Metrics

	// AcquirePermit attempts to acquire a permit for an execution at the priority or level contained in the context,
	// returning ErrExceeded if one could not be acquired. A priority must be stored in the context using the PriorityKey,
	// or a level must be stored in the context using the LevelKey. The priority or level must be greater than the current
	// rejection threshold for admission. Levels must be between 0 and 499.
	//
	// Example usage:
	//   ctx := priority.ContextWithPriority(context.Background(), priority.High)
	//   permit, err := throttler.AcquirePermit(ctx)
	AcquirePermit(ctx context.Context) error

	// AcquirePermitWithPriority attempts to acquire a permit for an execution at the given priority, returning ErrExceeded
	// if one could not be acquired. The priority must be greater than the current rejection threshold for
	// admission.
	AcquirePermitWithPriority(priority priority.Priority) error

	// AcquirePermitWithLevel attempts to acquire a permit for an execution at the given level, returning ErrExceeded if one
	// could not be acquired. The level must be greater than the current rejection threshold for admission.
	AcquirePermitWithLevel(level int) error

	// CanAcquirePermit returns whether it's possible to acquire a permit for an execution at the priority or level
	// contained in the context. A priority must be stored in the context using the PriorityKey, or a level must be stored
	// in the context using the LevelKey. The priority or level must be greater than the current rejection threshold for
	// admission. Levels must be between 0 and 499.
	CanAcquirePermit(ctx context.Context) bool

	// CanAcquirePermitWithPriority returns whether it's currently possible to acquire a permit for the priority.
	CanAcquirePermitWithPriority(priority priority.Priority) bool

	// CanAcquirePermitWithLevel returns whether it's currently possible to acquire a permit for the level. The level must
	// be between 0 and 499.
	CanAcquirePermitWithLevel(level int) bool

	// RecordResult records an execution result as a success or failure based on the failure handling configuration.
	RecordResult(result R)

	// RecordError records an error as a success or failure based on the failure handling configuration.
	RecordError(err error)

	// RecordSuccess records an execution success.
	RecordSuccess()

	// RecordFailure records an execution failure.
	RecordFailure()
}

type priorityThrottler[R any] struct {
	*adaptiveThrottler[R]
	prioritizer *priority.BasePrioritizer[*throttlerStats]
}

func (t *priorityThrottler[R]) AcquirePermit(ctx context.Context) error {
	return t.AcquirePermitWithLevel(priority.LevelForContext(ctx))
}

func (t *priorityThrottler[R]) AcquirePermitWithPriority(priority priority.Priority) error {
	return t.AcquirePermitWithLevel(priority.RandomLevel())
}

func (t *priorityThrottler[R]) AcquirePermitWithLevel(level int) error {
	if !t.CanAcquirePermitWithLevel(level) {
		return ErrExceeded
	}

	t.prioritizer.LevelTracker.RecordLevel(level)
	return nil
}

func (t *priorityThrottler[R]) CanAcquirePermit(ctx context.Context) bool {
	return t.CanAcquirePermitWithLevel(priority.LevelForContext(ctx))
}

func (t *priorityThrottler[R]) CanAcquirePermitWithPriority(priority priority.Priority) bool {
	return t.CanAcquirePermitWithLevel(priority.RandomLevel())
}

func (t *priorityThrottler[R]) CanAcquirePermitWithLevel(level int) bool {
	return level >= t.prioritizer.RejectionThreshold()
}

func (t *priorityThrottler[R]) RejectionRate() float64 {
	return t.prioritizer.RejectionRate()
}

func (t *priorityThrottler[R]) ToExecutor(_ R) any {
	pte := &priorityExecutor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{
			BaseFailurePolicy: t.BaseFailurePolicy,
		},
		priorityThrottler: t,
	}
	pte.Executor = pte
	return pte
}

// Implements Stats for throttler statistics.
type throttlerStats struct {
	executions       float64
	rejectionRate    float64
	maxRejectionRate float64
}

func (s *throttlerStats) ComputeRejectionRate() float64 {
	return min(s.rejectionRate, s.maxRejectionRate)
}

func (s *throttlerStats) DebugLogArgs() []any {
	return nil
}

// Must be locked externally
func (t *priorityThrottler[R]) getThrottlerStats() *throttlerStats {
	return &throttlerStats{
		executions: float64(t.ExecutionCount()),
		rejectionRate: computeRejectionRate(
			float64(t.ExecutionCount()),
			float64(t.SuccessCount()),
			t.successRateThreshold,
			t.maxRejectionRate,
		),
		maxRejectionRate: t.maxRejectionRate,
	}
}
