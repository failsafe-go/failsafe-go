package adaptivelimiter

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/priority"
)

// Tests that a rejection rate is computed as expected based on queue sizes.
func TestPrioritizer_Calibrate(t *testing.T) {
	p := NewPrioritizer()
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
	assert.True(t, p.RejectionThreshold() > 0 && p.RejectionThreshold() < 200, "low priority execution should be rejected")
}

func TestPrioritizer_WithLogger(t *testing.T) {
	// Given
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	p := NewPrioritizerBuilder().WithLogger(logger).Build()

	// When
	p.Calibrate()

	// Then
	assert.Contains(t, buf.String(), "prioritizer calibration")
}

func TestPrioritizer_ScheduleCalibrations(t *testing.T) {
	// Given
	p := NewPrioritizer()
	limiter := NewBuilder[any]().
		WithLimits(1, 1, 1).
		WithQueueing(1, 2).
		BuildPrioritized(p)
	shouldAcquireWithPriority(t, limiter, priority.Low)    // fill the limiter
	go shouldAcquireWithPriority(t, limiter, priority.Low) // fill the queue
	go shouldAcquireWithPriority(t, limiter, priority.Low) // fill the queue
	assertQueued(t, limiter, 2)

	// When
	cancel := p.ScheduleCalibrations(context.Background(), 10*time.Millisecond)
	defer cancel()
	// Wait for calibration
	time.Sleep(50 * time.Millisecond)

	// Then
	assert.Greater(t, p.RejectionRate(), 0.0)
	assert.GreaterOrEqual(t, p.RejectionThreshold(), 100)
	assert.LessOrEqual(t, p.RejectionThreshold(), 199)
}

func TestQueueRejectionStrategy_CombineStats(t *testing.T) {
	strategy := &queueRejectionStrategy{}
	statsFuncs := []func() *queueStats{
		func() *queueStats {
			return &queueStats{
				limit:              5,
				queued:             1,
				rejectionThreshold: 8,
				maxQueue:           10,
			}
		},
		func() *queueStats {
			return &queueStats{
				limit:              15,
				queued:             4,
				rejectionThreshold: 20,
				maxQueue:           25,
			}
		},
		func() *queueStats {
			return &queueStats{
				limit:              25,
				queued:             8,
				rejectionThreshold: 35,
				maxQueue:           50,
			}
		},
	}

	result := strategy.CombineStats(statsFuncs)

	assert.Equal(t, 45, result.limit)
	assert.Equal(t, 13, result.queued)
	assert.Equal(t, 63, result.rejectionThreshold)
	assert.Equal(t, 85, result.maxQueue)
}
