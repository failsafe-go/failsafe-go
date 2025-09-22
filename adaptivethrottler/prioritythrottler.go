package adaptivethrottler

import (
	"context"
	"math/rand"

	"github.com/failsafe-go/failsafe-go"
	priorityInternal "github.com/failsafe-go/failsafe-go/internal/priority"
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

	// TryAcquirePermit attempts to acquire a permit for an execution at the priority or level contained in the context,
	// returning one could be acquired. A priority must be stored in the context using the PriorityKey, or a level must be
	// stored in the context using the LevelKey. The priority or level must be greater than the current rejection threshold
	// for admission. Levels must be between 0 and 499.
	//
	// Example usage:
	//   ctx := priority.ContextWithPriority(context.Background(), priority.High)
	//   permit, err := throttler.AcquirePermit(ctx)
	TryAcquirePermit(ctx context.Context) bool

	// TryAcquirePermitWithPriority attempts to acquire a permit for an execution at the given priority, returning whether
	// one could be acquired. The priority must be greater than the current rejection threshold for admission.
	TryAcquirePermitWithPriority(priority priority.Priority) bool

	// TryAcquirePermitWithLevel attempts to acquire a permit for an execution at the given level, returning whether one
	// could be acquired. The level must be greater than the current rejection threshold for admission.
	TryAcquirePermitWithLevel(level int) bool

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
	prioritizer *priorityInternal.BasePrioritizer[*throttlerStats]
}

func (t *priorityThrottler[R]) AcquirePermit(ctx context.Context) error {
	level := priority.LevelFromContext(ctx)
	if level == -1 {
		level = 0
	}
	return t.AcquirePermitWithLevel(level)
}

func (t *priorityThrottler[R]) AcquirePermitWithPriority(priority priority.Priority) error {
	return t.AcquirePermitWithLevel(priority.RandomLevel())
}

func (t *priorityThrottler[R]) AcquirePermitWithLevel(level int) error {
	// Try to acquire through prioritizer
	if level >= t.prioritizer.RejectionThreshold() {
		t.prioritizer.LevelTracker.RecordLevel(level)
		return nil
	}

	// Maintain min flow to prevent starvation
	if rand.Float64() < 1.0-t.maxRejectionRate {
		t.prioritizer.LevelTracker.RecordLevel(level)
		return nil
	}

	return ErrExceeded
}

func (t *priorityThrottler[R]) TryAcquirePermit(ctx context.Context) bool {
	return t.AcquirePermit(ctx) == nil
}

func (t *priorityThrottler[R]) TryAcquirePermitWithPriority(priority priority.Priority) bool {
	return t.AcquirePermitWithPriority(priority) == nil
}

func (t *priorityThrottler[R]) TryAcquirePermitWithLevel(level int) bool {
	return t.AcquirePermitWithLevel(level) == nil
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
			t.executionThreshold,
		),
		maxRejectionRate: t.maxRejectionRate,
	}
}
