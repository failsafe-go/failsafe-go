package adaptivelimiterold

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

type Priority int

const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
	PriorityCritical
)

// priorityRange provides a wider range of priorities that allow for rejecting a subset of requests within a Priority.
type priorityRange struct {
	lower, upper int
}

// Defining the priority ranges as a map
var priorityRanges = map[Priority]priorityRange{
	PriorityLow:      {0, 99},
	PriorityMedium:   {100, 199},
	PriorityHigh:     {200, 299},
	PriorityCritical: {300, 399},
}

func generateGranularPriority(priority Priority) int {
	r := priorityRanges[priority]
	return rand.Intn(r.upper-r.lower+1) + r.lower
}

// Prioritizer regularly adjusts a rejection threshold for incoming requests based on throughput of priority limiters.
type Prioritizer[R any] interface {
	register(limiter *priorityBlockingLimiter[R])

	// Threshold returns the current granular priority threshold, below which requests will be rejected. A granular priority
	// for each request is generated based on its given priority.
	Threshold() int

	// ScheduleCalibrations runs calibration on an interval until the ctx is done or the returned CancelFunc is called.
	ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc
}

type prioritizer[R any] struct {
	logger *slog.Logger
	kp     float64
	ki     float64

	priorityThreshold atomic.Int32
	mu                sync.Mutex
	limiters          []*priorityBlockingLimiter[R] // Guarded by mu
	calibrations      *calibrationWindow            // Guarded by mu
}

func NewPrioritizer[R any]() Prioritizer[R] {
	return &prioritizer[R]{
		// kp: 0.1, // Gradual response to spikes
		// ki: 1.4, // Aggressive response to sustained load
		kp: 0.05, // Reduced from 0.1
		ki: 0.3,  // Reduced from 1.4
		calibrations: &calibrationWindow{
			window: make([]calibrationPeriod, 30), // 30 second history
			// integralEWMA: util.NewEWMA(30, 5),           // 30 samples, 5 warmup
		},
	}
}

func (p *prioritizer[R]) register(limiter *priorityBlockingLimiter[R]) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logger = limiter.adaptiveLimiter.logger // TODO remove this hack
	p.limiters = append(p.limiters, limiter)
}

func (p *prioritizer[R]) Threshold() int {
	return int(p.priorityThreshold.Load())
}

// ScheduleCalibrations runs calibration on an interval
func (p *prioritizer[R]) ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				p.Calibrate()
			}
		}
	}()

	return func() {
		close(done)
	}
}

// Calibrate adjusts the priority threshold based on request flow metrics
func (p *prioritizer[R]) Calibrate() {
	p.mu.Lock()

	// Reset limiter stats and find the most overloaded limiter
	var maxRatio float32
	var maxIn, maxOut int
	var mostOverloaded *priorityBlockingLimiter[R]
	for _, limiter := range p.limiters {
		inCount := int(limiter.inCount.Swap(0))
		outCount := int(limiter.outCount.Swap(0))
		ratio := float32(inCount) / float32(outCount)
		if mostOverloaded == nil || ratio > maxRatio {
			mostOverloaded = limiter
			maxRatio = ratio
			maxIn = inCount
			maxOut = outCount
		}
	}
	if mostOverloaded == nil {
		p.mu.Unlock()
		return
	}

	// Get latest error and integral sum from calibration window
	freeInflight := mostOverloaded.Limit() - mostOverloaded.Inflight()
	errorValue, integralSum := p.calibrations.add(maxIn, maxOut, freeInflight, mostOverloaded.Limit())
	p.mu.Unlock()

	// Calculate PI
	adjustment := p.kp*errorValue + p.ki*integralSum
	rejectionRatio := math.Max(0, math.Min(1, adjustment))

	if p.logger != nil && p.logger.Enabled(nil, slog.LevelDebug) {
		p.logger.Debug("prioritizer calibration",
			// "newThresh",,
			"oldThresh", p.priorityThreshold.Load(),
			"in", maxIn,
			"out", maxOut,
			"freeInflight", freeInflight,
			"error", fmt.Sprintf("%.2f", errorValue),
			"integral", fmt.Sprintf("%.2f", integralSum),
			"PI", fmt.Sprintf("%.2f", adjustment),
			"rejectionRatio", fmt.Sprintf("%.2f", rejectionRatio),
		)
	}

	// p.priorityThreshold.Store(int32(newThreshold))
}

type calibrationWindow struct {
	window      []calibrationPeriod
	size        int
	head        int
	integralSum float64 // Sum of P values over the window
	// integralEWMA util.MovingAverage
}

type calibrationPeriod struct {
	inCount  int     // Items that entered the limiter during the calibration period
	outCount int     // Items that exited the limiter during the calibration period
	error    float64 // The computed P value for the calibration period
}

func (c *calibrationWindow) add(in, out, freeInflight int, limit int) (error float64, integralSum float64) {
	if c.size < len(c.window) {
		c.size++
	} else {
		// If window is full, subtract the oldest error before overwriting
		c.integralSum -= c.window[c.head].error
	}

	error = computeError(in, out, freeInflight, limit)
	c.integralSum += error
	c.window[c.head] = calibrationPeriod{
		inCount:  in,
		outCount: out,
		error:    error,
	}
	c.head = (c.head + 1) % len(c.window)

	return error, c.integralSum
}

// Computes the error from execution flow metrics.
// A positive error value indicates overload, negative indicates underload.
func computeError(in, out, freeInflight int, limit int) float64 {
	normalizer := out
	if normalizer == 0 {
		normalizer = limit
	}
	numerator := float64(in - (out + freeInflight))
	return numerator / float64(normalizer)
}
