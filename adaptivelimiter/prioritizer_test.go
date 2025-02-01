package adaptivelimiter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Tests that a rejection rate is computed as expected based on queue in/out stats.
func TestPrioritizer_Calibrate(t *testing.T) {
	p := NewPrioritizer().(*prioritizer)
	limiter := NewBuilder[any]().
		WithLimits(1, 10, 1).
		WithShortWindow(time.Second, time.Second, 10).
		WithRejectionFactors(2, 4).
		BuildPrioritized(p).(*priorityLimiter[any])

	acquireBlocking := func() {
		go limiter.AcquirePermit(context.Background(), PriorityLow)
	}
	assertBlocked := func(blocked int) {
		require.Eventually(t, func() bool {
			return limiter.Blocked() == blocked
		}, 300*time.Millisecond, 10*time.Millisecond)
	}

	permit, err := limiter.AcquirePermit(context.Background(), PriorityLow)
	require.NoError(t, err)
	require.Equal(t, 1, int(limiter.inCount.Load()))
	require.Equal(t, 1, int(limiter.outCount.Load()))
	acquireBlocking()
	assertBlocked(1)
	acquireBlocking()
	assertBlocked(2)
	acquireBlocking()
	assertBlocked(3)
	acquireBlocking()
	assertBlocked(4)
	permit.Record()
	require.Equal(t, 5, int(limiter.inCount.Load()))
	// Wait for blocked permit to be acquired
	require.Eventually(t, func() bool {
		return limiter.outCount.Load() == 2
	}, 300*time.Millisecond, 10*time.Millisecond)

	p.Calibrate()
	require.Equal(t, .5, limiter.RejectionRate())
	require.True(t, p.priorityThreshold.Load() > 0 && p.priorityThreshold.Load() < 200, "low priority execution should be rejected")
}
