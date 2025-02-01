package adaptivelimiter

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test asserts that blocking requests block or are rejected and the rejection rate is updated as expected.
func TestBlockingLimiter_AcquirePermit(t *testing.T) {
	rand.Seed(1) // Force consistency with granular priorities
	limiter := NewBuilder[any]().WithLimits(1, 10, 1).WithRejectionFactors(2, 4).Build()
	_, err := limiter.AcquirePermit(context.Background())
	require.NoError(t, err)

	acquireBlocking := func() {
		go limiter.AcquirePermit(context.Background())
	}
	assertBlocked := func(blocked int) {
		require.Eventually(t, func() bool {
			return limiter.Blocked() == blocked
		}, 300*time.Millisecond, 10*time.Millisecond)
	}

	acquireBlocking()
	assertBlocked(1)
	acquireBlocking()
	assertBlocked(2)
	acquireBlocking()
	assertBlocked(3)
	acquireBlocking()
	assertBlocked(4)
	assert.Equal(t, .5, limiter.RejectionRate())

	// Queue is full
	permit, err := limiter.AcquirePermit(context.Background())
	require.Nil(t, permit)
	require.ErrorIs(t, err, ErrExceeded)
	assertBlocked(4)
	assert.Equal(t, 1.0, limiter.RejectionRate())
}

func TestBlockingLimiter_computeRejectionRate(t *testing.T) {
	tests := []struct {
		name               string
		queueSize          int
		rejectionThreshold int
		maxQueueSize       int
		expectedRate       float64
	}{
		{
			name:               "Below threshold returns 0",
			queueSize:          5,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       0,
		},
		{
			name:               "Above max returns 1",
			queueSize:          25,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       1,
		},
		{
			name:               "Mid-range returns proportional rate",
			queueSize:          15,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       0.5,
		},
		{
			name:               "Equal to threshold returns 0",
			queueSize:          10,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       0,
		},
		{
			name:               "Equal to max returns 1",
			queueSize:          20,
			rejectionThreshold: 10,
			maxQueueSize:       20,
			expectedRate:       1,
		},
		{
			name:               "One above threshold",
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
