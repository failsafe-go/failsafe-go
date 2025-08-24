package adaptivelimiter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// This test asserts that queued requests block or are rejected and the rejection rate is updated as expected.
func TestQueueingLimiter_AcquirePermit(t *testing.T) {
	// Given
	limiter := createQueueingLimiter(t, 3, 3)

	// When
	permit, err := limiter.AcquirePermit(context.Background())

	// Then
	assert.Nil(t, permit)
	assert.ErrorIs(t, err, ErrExceeded)
	assertQueued(t, limiter, 3)
	assert.Equal(t, 1.0, limiter.computeRejectionRate())
}

func TestQueueingLimiter_CanAcquirePermit(t *testing.T) {
	// Given
	limiter := createQueueingLimiter(t, 3, 2)
	assert.True(t, limiter.CanAcquirePermit())

	// When
	acquireAsync(limiter)

	// Then
	assertQueued(t, limiter, 3)
	assert.False(t, limiter.CanAcquirePermit())
	assert.Equal(t, 1.0, limiter.computeRejectionRate())
}

func createQueueingLimiter(t *testing.T, queueCapacity float32, queueSize int) *queueingLimiter[any] {
	limiter := NewBuilder[any]().
		WithLimits(1, 10, 1).
		WithQueueing(queueCapacity, queueCapacity).
		Build().(*queueingLimiter[any])

	// Fill the limiter
	limiter.TryAcquirePermit()

	// Fill the queue
	for i := 0; i < queueSize; i++ {
		acquireAsync(limiter)
		assertQueued(t, limiter, i+1)
	}
	return limiter
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
