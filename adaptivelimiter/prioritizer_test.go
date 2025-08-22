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

	acquireBlocking := func() {
		go limiter.AcquirePermitWithPriority(context.Background(), PriorityLow)
	}
	assertQueued := func(queued int) {
		assert.Eventually(t, func() bool {
			return limiter.Queued() == queued
		}, 300*time.Millisecond, 10*time.Millisecond)
	}

	permit, err := limiter.AcquirePermitWithPriority(context.Background(), PriorityLow)
	assert.NoError(t, err)
	acquireBlocking()
	assertQueued(1)
	acquireBlocking()
	assertQueued(2)
	acquireBlocking()
	assertQueued(3)
	acquireBlocking()
	assertQueued(4)
	permit.Record()

	p.Calibrate()
	assert.Equal(t, .5, p.RejectionRate())
	assert.True(t, p.levelThreshold.Load() > 0 && p.levelThreshold.Load() < 200, "low priority execution should be rejected")
}

func TestComputeRejectionRate(t *testing.T) {
	tests := []struct {
		name               string
		queueSize          int
		rejectionThreshold int
		maxQueueSize       int
		expectedRate       float64
	}{
		{
			name:               "queueSize below rejectionThreshold",
			queueSize:          50,
			rejectionThreshold: 60,
			maxQueueSize:       100,
			expectedRate:       0,
		},
		{
			name:               "queueSize equal to rejectionThreshold",
			queueSize:          60,
			rejectionThreshold: 60,
			maxQueueSize:       100,
			expectedRate:       0,
		},
		{
			name:               "queueSize between rejectionThreshold and maxQueueSize",
			queueSize:          80,
			rejectionThreshold: 60,
			maxQueueSize:       100,
			expectedRate:       .5,
		},
		{
			name:               "queueSize equal to maxQueueSize",
			queueSize:          100,
			rejectionThreshold: 60,
			maxQueueSize:       100,
			expectedRate:       1,
		},
		{
			name:               "queueSize above maxQueueSize",
			queueSize:          120,
			rejectionThreshold: 60,
			maxQueueSize:       100,
			expectedRate:       1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rate := computeRejectionRate(tc.queueSize, tc.rejectionThreshold, tc.maxQueueSize)
			assert.Equal(t, tc.expectedRate, rate)
		})
	}
}
