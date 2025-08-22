package adaptivelimiter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// This test asserts that queued requests block or are rejected and the rejection rate is updated as expected.
func TestQueueingLimiter_AcquirePermit(t *testing.T) {
	limiter := NewBuilder[any]().
		WithLimits(1, 10, 1).
		WithQueueing(2, 4).
		Build().(*queueingLimiter[any])
	_, err := limiter.AcquirePermit(context.Background())
	assert.NoError(t, err)

	acquireBlocking := func() {
		go limiter.AcquirePermit(context.Background())
	}
	assertQueued := func(queued int) {
		assert.Eventually(t, func() bool {
			return limiter.Queued() == queued
		}, 300*time.Millisecond, 10*time.Millisecond)
	}

	go acquireBlocking()
	assertQueued(1)
	go acquireBlocking()
	assertQueued(2)
	go acquireBlocking()
	assertQueued(3)
	assert.Equal(t, .5, limiter.computeRejectionRate())
	go acquireBlocking()
	assertQueued(4)
	assert.Equal(t, 1.0, limiter.computeRejectionRate())

	// Queue is full
	permit, err := limiter.AcquirePermit(context.Background())
	assert.Nil(t, permit)
	assert.ErrorIs(t, err, ErrExceeded)
	assertQueued(4)
	assert.Eventually(t, func() bool {
		return limiter.computeRejectionRate() == 1.0
	}, 300*time.Millisecond, 10*time.Millisecond)
}

func TestQueueingLimiter_computeRejectionRate(t *testing.T) {
	tests := []struct {
		name               string
		queueSize          int
		rejectionThreshold int
		maxQueueSize       int
		expectedRate       float64
	}{
		{
			name:               "below threshold returns 0",
			queueSize:          5,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       0,
		},
		{
			name:               "above max returns 1",
			queueSize:          25,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       1,
		},
		{
			name:               "mid-range returns proportional rate",
			queueSize:          15,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       0.5,
		},
		{
			name:               "equal to threshold returns 0",
			queueSize:          10,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       0,
		},
		{
			name:               "equal to max returns 1",
			queueSize:          20,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       1,
		},
		{
			name:               "one above threshold",
			queueSize:          11,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       0.1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := computeRejectionRate(tc.queueSize, tc.rejectionThreshold, tc.maxQueueSize)
			assert.InDelta(t, tc.expectedRate, actual, 0.0001)
		})
	}
}
