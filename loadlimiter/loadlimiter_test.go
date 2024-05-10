package loadlimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestFailureSignal(t *testing.T) {
	// Given
	ll := With[any](NewFailureSignal[any]().
		WithFailureRateThreshold(50, 3, 200*time.Millisecond).
		HandleResult(false),
		NewShedAllStrategy())

	// When / Then
	ll.AcquirePermit()

	executor.Get(testutil.GetFalseFn)
	executor.Get(testutil.GetTrueFn)
	// Force results to roll off
	time.Sleep(210 * time.Millisecond)
	executor.Get(testutil.GetFalseFn)
	executor.Get(testutil.GetTrueFn)
	// Force result to another bucket
	time.Sleep(50 * time.Millisecond)
	executor.Get(testutil.GetTrueFn)
	assert.True(t, cb.IsClosed())
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsOpen())
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsHalfOpen())
	executor.Get(testutil.GetFalseFn)
	// Half-open -> Open
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsOpen())
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsHalfOpen())
	executor.Get(testutil.GetTrueFn)
	// Half-open -> close
	executor.Get(testutil.GetTrueFn)
	assert.True(t, cb.IsClosed())
}
