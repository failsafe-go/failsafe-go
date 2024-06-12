package circuitbreaker

import (
	"testing"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/testutil"

	"github.com/stretchr/testify/assert"
)

var _ circuitState[any] = &openState[any]{}

func TestPartiallyOpenState_TryAcquirePermit(t *testing.T) {
	freq := 10
	breaker := WithDefaults[any]().(*circuitBreaker[any])
	breaker.config.percentageAllowedExecutions = uint(100 / freq)
	breaker.open(testutil.TestExecution[any]{})
	assert.True(t, breaker.IsOpen())
	for i := 0; i < 2*freq; i++ {
		if i%freq == 0 {
			assert.True(t, breaker.TryAcquirePermit())
		} else {
			assert.False(t, breaker.TryAcquirePermit())
		}
	}
	assert.True(t, breaker.IsOpen())
}

func TestPartiallyOpenState_WithFailureThreshold(t *testing.T) {
	// Only 50% of the executions will be allowed when the circuit is open, i.e., 1 in 2 executions will be denied.
	percentageAllowedExecutions := uint(50)
	breaker := Builder[any]().
		WithFailureThreshold(2).
		WithPercentageAllowedExecutions(percentageAllowedExecutions).
		Build().(*circuitBreaker[any])

	assert.True(t, breaker.IsClosed())

	breaker.RecordFailure()
	assert.True(t, breaker.IsClosed())
	assert.Equal(t, uint(0), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(1), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(1), breaker.state.getStats().getExecutionCount())

	breaker.RecordFailure()
	assert.True(t, breaker.IsOpen())
	assert.Equal(t, uint(0), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getExecutionCount())

	// The first execution after opening the circuit is allowed, but the circuit remains open.
	assert.True(t, breaker.TryAcquirePermit())
	breaker.RecordFailure()

	assert.True(t, breaker.IsOpen())
	assert.Equal(t, uint(0), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getExecutionCount())

	// The second execution after opening the circuit is denied.
	assert.False(t, breaker.TryAcquirePermit())

	assert.True(t, breaker.IsOpen())
	assert.Equal(t, uint(0), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getExecutionCount())

	// The third execution after opening the circuit is allowed, and the circuit gets closed.
	assert.True(t, breaker.TryAcquirePermit())
	breaker.RecordSuccess()

	assert.True(t, breaker.IsClosed())
	assert.Equal(t, uint(0), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(0), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(0), breaker.state.getStats().getExecutionCount())
}

func TestPartiallyOpenState_WithFailureThresholdRatio(t *testing.T) {
	// Only 50% of the executions will be allowed when the circuit is open, i.e., 1 in 2 executions will be denied.
	percentageAllowedExecutions := uint(50)

	breaker := Builder[any]().
		WithFailureThresholdRatio(2, 4).
		WithPercentageAllowedExecutions(percentageAllowedExecutions).
		Build().(*circuitBreaker[any])

	assert.True(t, breaker.IsClosed())

	breaker.RecordFailure()
	assert.True(t, breaker.IsClosed())
	assert.Equal(t, uint(0), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(1), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(1), breaker.state.getStats().getExecutionCount())

	breaker.RecordFailure()
	assert.True(t, breaker.IsOpen())
	assert.Equal(t, uint(0), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getExecutionCount())

	// The first execution after opening the circuit is allowed, but the circuit remains open.
	assert.True(t, breaker.TryAcquirePermit())
	breaker.RecordSuccess()

	assert.True(t, breaker.IsOpen())
	assert.Equal(t, uint(1), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(3), breaker.state.getStats().getExecutionCount())

	// The second execution after opening the circuit is denied.
	assert.False(t, breaker.TryAcquirePermit())

	assert.True(t, breaker.IsOpen())
	assert.Equal(t, uint(1), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(3), breaker.state.getStats().getExecutionCount())

	// The third execution after opening the circuit is allowed, but the circuit remains open.
	assert.True(t, breaker.TryAcquirePermit())
	breaker.RecordSuccess()

	assert.True(t, breaker.IsOpen())
	assert.Equal(t, uint(2), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(4), breaker.state.getStats().getExecutionCount())

	// The fourth execution after opening the circuit is denied.
	assert.False(t, breaker.TryAcquirePermit())

	assert.True(t, breaker.IsOpen())
	assert.Equal(t, uint(2), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(2), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(4), breaker.state.getStats().getExecutionCount())

	// The fifth execution after opening the circuit is allowed, and the circuit gets closed.
	assert.True(t, breaker.TryAcquirePermit())
	breaker.RecordSuccess()

	assert.True(t, breaker.IsClosed())
	assert.Equal(t, uint(0), breaker.state.getStats().getSuccessCount())
	assert.Equal(t, uint(0), breaker.state.getStats().getFailureCount())
	assert.Equal(t, uint(0), breaker.state.getStats().getExecutionCount())
}

func TestPartiallyOpenState_NoRemainingDelay(t *testing.T) {
	breaker := Builder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		return 10 * time.Millisecond
	}).Build().(*circuitBreaker[any])
	assert.Equal(t, time.Duration(0), breaker.RemainingDelay())

	// When
	breaker.open(testutil.TestExecution[any]{})
	assert.True(t, breaker.RemainingDelay() > 0)
	time.Sleep(50 * time.Millisecond)

	// Then
	assert.Equal(t, time.Duration(0), breaker.RemainingDelay())
}
