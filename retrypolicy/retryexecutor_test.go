package retrypolicy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestAdjustForBackoff(t *testing.T) {
	// Given
	rpc := Builder[any]().WithBackoff(time.Second, 10*time.Second).(*retryPolicyConfig[any])
	exec := &testutil.TestExecution[any]{
		TheAttempts: 1,
	}
	delay := rpc.Delay
	f := func() time.Duration {
		delay = adjustForBackoff(rpc, exec, delay)
		exec.TheAttempts++
		return delay
	}

	// When / Then
	assert.Equal(t, time.Second, f())
	assert.Equal(t, 2*time.Second, f())
	assert.Equal(t, 4*time.Second, f())
	assert.Equal(t, 8*time.Second, f())
	assert.Equal(t, 10*time.Second, f())
}
