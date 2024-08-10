package retrypolicy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestGetFixedOrRandomDelay(t *testing.T) {
	// Given
	rpc := Builder[any]().WithBackoffFactor(2*time.Second, 30*time.Second, 2).(*config[any])
	rpe := &executor[any]{
		retryPolicy: &retryPolicy[any]{
			config: rpc,
		},
	}
	exec := &testutil.TestExecution[any]{}
	delay := rpc.Delay
	f := func() time.Duration {
		delay = rpe.getFixedOrRandomDelay(exec)
		exec.TheRetries++
		return delay
	}

	// When / Then
	assert.Equal(t, 2*time.Second, f())
	assert.Equal(t, 4*time.Second, f())
	assert.Equal(t, 8*time.Second, f())
	assert.Equal(t, 16*time.Second, f())
	assert.Equal(t, 30*time.Second, f())
}
