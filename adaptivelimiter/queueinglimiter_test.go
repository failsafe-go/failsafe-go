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
		WithQueueing(3, 3).
		Build().(*queueingLimiter[any])
	_, err := limiter.AcquirePermit(context.Background())
	assert.NoError(t, err)

	acquire := func() {
		go limiter.AcquirePermit(context.Background())
	}
	assertQueued := func(queued int) {
		assert.Eventually(t, func() bool {
			return limiter.Queued() == queued
		}, 300*time.Millisecond, 10*time.Millisecond)
	}

	acquire()
	assertQueued(1)
	acquire()
	assertQueued(2)
	acquire()
	assertQueued(3)
	assert.Equal(t, 1.0, limiter.computeRejectionRate())

	// Queue is full
	permit, err := limiter.AcquirePermit(context.Background())
	assert.Nil(t, permit)
	assert.ErrorIs(t, err, ErrExceeded)
	assertQueued(3)
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
			name:               "queueSize below rejectionThreshold",
			queueSize:          5,
			rejectionThreshold: 10,
			maxQueueSize:       20,
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
		{
			name:               "queueSize above rejectionThreshold",
			queueSize:          11,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       0.1,
		},
		{
			name:               "queueSize equalToRejectionThreshold and maxQueueSize",
			queueSize:          3,
			rejectionThreshold: 3,
			maxQueueSize:       3,
			expectedRate:       1,
		},
		{
			name:               "queueSize below rejectionThreshold and equal to maxQueueSize",
			queueSize:          2,
			rejectionThreshold: 3,
			maxQueueSize:       3,
			expectedRate:       0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := computeRejectionRate(tc.queueSize, tc.rejectionThreshold, tc.maxQueueSize)
			assert.InDelta(t, tc.expectedRate, actual, 0.0001)
		})
	}
}
