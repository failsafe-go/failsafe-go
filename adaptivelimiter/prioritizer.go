package adaptivelimiter

import (
	"context"
	"fmt"
	"log/slog"
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

type Prioritizer[R any] interface {
	register(limiter *priorityBlockingLimiter[R])

	// CurrentPriority returns the current priority threshold, below which requests will be rejected.
	CurrentPriority() Priority

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
		kp: 0.1, // Gradual response to spikes
		ki: 1.4, // Aggressive response to sustained load
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

func (p *prioritizer[R]) CurrentPriority() Priority {
	return Priority(p.priorityThreshold.Load())
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
		if ratio > maxRatio {
			mostOverloaded = limiter
			maxRatio = ratio
			maxIn = inCount
			maxOut = outCount
		}
	}
	if mostOverloaded == nil {
		return
	}

	// Get both P value and integral sum from calibration window
	freeInflight := mostOverloaded.Limit() - mostOverloaded.Inflight()
	pValue, integralSum := p.calibrations.add(maxIn, maxOut, freeInflight, mostOverloaded.Limit())
	p.mu.Unlock()

	// Calculate PID adjustment
	adjustment := p.kp*pValue + p.ki*integralSum

	// Convert to threshold change
	currentThreshold := Priority(p.priorityThreshold.Load())
	newThreshold := currentThreshold

	// Only change threshold on significant adjustments
	if adjustment > 0.5 && currentThreshold < PriorityCritical {
		// Overloaded, increase the threshold
		newThreshold++
	} else if adjustment < -0.5 && currentThreshold > PriorityLow {
		// Underloaded, lower the threshold
		newThreshold--
	}

	if p.logger != nil && newThreshold != currentThreshold {
		p.logger.Debug("updated priority threshold",
			"newThresh", newThreshold,
			"oldThresh", currentThreshold,
			"pValue", fmt.Sprintf("%.2f", pValue),
			"integral", fmt.Sprintf("%.2f", integralSum),
			"adjustment", fmt.Sprintf("%.2f", adjustment),
			"maxIn", maxIn,
			"maxOut", maxOut)
	}

	p.priorityThreshold.Store(int32(newThreshold))
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
	pValue   float64 // The computed P value for the calibration period
}

func (c *calibrationWindow) add(in, out, freeInflight int, limit int) (pValue float64, integralSum float64) {
	if c.size < len(c.window) {
		c.size++
	} else {
		// If window is full, subtract the oldest P value before overwriting
		c.integralSum -= c.window[c.head].pValue
	}

	pValue = computePValue(in, out, freeInflight, limit)
	c.integralSum += pValue
	c.window[c.head] = calibrationPeriod{
		inCount:  in,
		outCount: out,
		pValue:   pValue,
	}
	c.head = (c.head + 1) % len(c.window)

	return pValue, c.integralSum
}

// Computes P value from request flow metrics.
// A positive P value indicates overload, negative indicates underload.
func computePValue(in, out, freeInflight int, limit int) float64 {
	if out == 0 {
		return float64(limit)
	}
	return float64(in-(out+freeInflight)) / float64(out)
}

// type calibrationWindow struct {
// 	window       []calibrationPeriod
// 	size         int
// 	head         int
// 	//integralEWMA util.MovingAverage
// }
//
// type calibrationPeriod struct {
// 	inCount  int
// 	outCount int
// 	pValue   float64
// }
//
// func (c *calibrationWindow) add(in, out, freeInflight int, limit int) (pValue float64, integralSum float64) {
// 	if c.size < len(c.window) {
// 		c.size++
// 	}
//
// 	// Calculate P value from request flow metrics
// 	pValue = computePValue(in, out, freeInflight, limit)
//
// 	// Use EWMA for integral term instead of unbounded sum
// 	// This provides a bounded history window and weights recent values more heavily
// 	//integralSum = c.integralEWMA.Add(pValue)
//
// 	c.window[c.head] = calibrationPeriod{
// 		inCount:  in,
// 		outCount: out,
// 		pValue:   pValue,
// 	}
// 	c.head = (c.head + 1) % len(c.window)
// 	return pValue, integralSum
// }
