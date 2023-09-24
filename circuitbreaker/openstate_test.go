package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

var _ circuitState[any] = &openState[any]{}

func TestTryAcquirePermit(t *testing.T) {
	breaker := Builder[any]().WithDelayFn(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		return 100 * time.Millisecond
	}).Build().(*circuitBreaker[any])
	breaker.open(testutil.TestExecution[any]{})
	assert.True(t, breaker.IsOpen())
	assert.False(t, breaker.TryAcquirePermit())

	// When
	time.Sleep(110 * time.Millisecond)

	// Then
	assert.True(t, breaker.TryAcquirePermit())
	assert.True(t, breaker.IsHalfOpen())
}
