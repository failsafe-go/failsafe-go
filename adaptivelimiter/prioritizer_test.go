package adaptivelimiter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/priority"
)

// Tests that a rejection rate is computed as expected based on queue sizes.
func TestPrioritizer_Calibrate(t *testing.T) {
	p := NewPrioritizer().(*prioritizer)
	limiter := NewBuilder[any]().
		WithLimits(1, 10, 1).
		WithRecentWindow(time.Second, time.Second, 10).
		WithQueueing(2, 4).
		BuildPrioritized(p).(*priorityLimiter[any])

	acquire := func() {
		go limiter.AcquirePermitWithPriority(context.Background(), priority.Low)
	}

	permit, err := limiter.AcquirePermitWithPriority(context.Background(), priority.Low)
	assert.NoError(t, err)
	acquire()
	assertQueued(t, limiter, 1)
	acquire()
	assertQueued(t, limiter, 2)
	acquire()
	assertQueued(t, limiter, 3)
	acquire()
	assertQueued(t, limiter, 4)
	permit.Record()

	p.Calibrate()
	assert.Equal(t, .5, p.RejectionRate())
	assert.True(t, p.rejectionThreshold.Load() > 0 && p.rejectionThreshold.Load() < 200, "low priority execution should be rejected")
}
