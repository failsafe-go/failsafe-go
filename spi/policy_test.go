package spi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe"
)

func TestShouldComputeDelay(t *testing.T) {
	expected := 5 * time.Millisecond
	policy := BaseDelayablePolicy[any]{
		DelayFn: func(exec *failsafe.Execution[any]) time.Duration {
			return expected
		},
	}

	assert.Equal(t, expected, policy.ComputeDelay(&failsafe.Execution[any]{
		LastResult: true,
	}))
	assert.Equal(t, time.Duration(-1), policy.ComputeDelay(nil))
}
