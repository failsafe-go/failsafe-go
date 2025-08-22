package adaptivelimiter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Tests that a rejection rate is computed as expected based on queue sizes.
func TestPrioritizer_Calibrate(t *testing.T) {
	p := NewPrioritizer().(*prioritizer)
	limiter := NewBuilder[any]().
		WithLimits(1, 10, 1).
		WithShortWindow(time.Second, time.Second, 10).
		WithQueueing(2, 4).
		BuildPrioritized(p).(*priorityLimiter[any])

	acquire := func() {
		go limiter.AcquirePermitWithPriority(context.Background(), PriorityLow)
	}
	assertQueued := func(queued int) {
		assert.Eventually(t, func() bool {
			return limiter.Queued() == queued
		}, 300*time.Millisecond, 10*time.Millisecond)
	}

	permit, err := limiter.AcquirePermitWithPriority(context.Background(), PriorityLow)
	assert.NoError(t, err)
	acquire()
	assertQueued(1)
	acquire()
	assertQueued(2)
	acquire()
	assertQueued(3)
	acquire()
	assertQueued(4)
	permit.Record()

	p.Calibrate()
	assert.Equal(t, .5, p.RejectionRate())
	assert.True(t, p.levelThreshold.Load() > 0 && p.levelThreshold.Load() < 200, "low priority execution should be rejected")
}
