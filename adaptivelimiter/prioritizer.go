package adaptivelimiter

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/influxdata/tdigest"
)

type Priority int

const (
	PriorityVeryLow Priority = iota
	PriorityLow
	PriorityMedium
	PriorityHigh
	PriorityVeryHigh
)

// priorityRange provides a wider range of priorities that allow for rejecting a subset of requests within a Priority.
type priorityRange struct {
	lower, upper int
}

// Defining the priority ranges as a map
var priorityRanges = map[Priority]priorityRange{
	PriorityVeryLow:  {0, 99},
	PriorityLow:      {100, 199},
	PriorityMedium:   {200, 299},
	PriorityHigh:     {300, 399},
	PriorityVeryHigh: {400, 499},
}

func generateGranularPriority(priority Priority) int {
	r := priorityRanges[priority]
	return rand.Intn(r.upper-r.lower+1) + r.lower
}

// Prioritizer computes a rejection rate and priority threshold for priority limiters, which can be used to control
// rejection of prioritized executions across multiple priority limiters.
type Prioritizer interface {
	// RejectionRate returns the current rate, from 0 to 1, at which the limiter will reject requests, based on recent
	// execution times.
	RejectionRate() float64

	// Calibrate calibrates the RejectionRate based on recent execution times from registered limiters.
	Calibrate()

	// ScheduleCalibrations runs calibration on the interval until the ctx is done or the returned CancelFunc is called.
	ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc

	register(limiter pidStats)
	recordPriority(priority int)
	threshold() int
}

// PrioritizerBuilder builds Prioritizer instances.
//
// This type is not concurrency safe.
type PrioritizerBuilder interface {
	// OnPriorityChanged configures a listener to be called with the priority threshold changes.
	OnPriorityChanged(listener func(event PriorityChangedEvent)) PrioritizerBuilder

	// WithLogger configures a logger which provides debug logging of calibrations.
	WithLogger(logger *slog.Logger) PrioritizerBuilder

	// Build returns a new Prioritizer using the builder's configuration.
	Build() Prioritizer
}

// PriorityChangedEvent indicates an Prioritizer's priority threshold has changed.
type PriorityChangedEvent struct {
	OldPriorityThreshold uint
	NewPriorityThreshold uint
}

type prioritizerConfig struct {
	logger   *slog.Logger
	kp       float64 // Proportional gain: responds to immediate load
	ki       float64 // Integral gain: responds to sustained load over time
	listener func(event PriorityChangedEvent)
}

var _ PrioritizerBuilder = &prioritizerConfig{}

func NewPrioritizer() Prioritizer {
	return NewPrioritizerBuilder().Build()
}

// NewPrioritizerBuilder returns a PrioritizerBuilder.
func NewPrioritizerBuilder() PrioritizerBuilder {
	return &prioritizerConfig{
		// Using a small value (.1) results in a gradual response to spikes
		// If P(t)=0.5 (50% overload), this kp value adds 0.05 to the rejection rate
		kp: .1, // .05,

		// Using a large value (1.4) results in aggressive response to sustained load
		// If sum(P)=1.0, this ki value adds 1.4 to the rejection rate
		ki: .14, // .3,
	}
}

func (c *prioritizerConfig) WithLogger(logger *slog.Logger) PrioritizerBuilder {
	c.logger = logger
	return c
}

func (c *prioritizerConfig) OnPriorityChanged(listener func(event PriorityChangedEvent)) PrioritizerBuilder {
	c.listener = listener
	return c
}

func (c *prioritizerConfig) Build() Prioritizer {
	pCopy := *c
	return &prioritizer{
		prioritizerConfig: &pCopy, // TODO copy base fields
		calibrations:      newPidCalibrationWindow(30),
		// integralEWMA:      util.NewEWMA(10, 5), // 30 sample window, 5 warmup samples
		digest: tdigest.NewWithCompression(100),
	}
}

type prioritizer struct {
	*prioritizerConfig

	// Mutable state
	priorityThreshold atomic.Int32
	mu                sync.Mutex
	limiters          []pidStats            // Guarded by mu
	calibrations      *pidCalibrationWindow // Guarded by mu
	// integralEWMA      util.MovingAverage
	digest        *tdigest.TDigest // Guarded by mu
	rejectionRate float64          // Guarded by mu
}

func (p *prioritizer) register(limiter pidStats) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.limiters = append(p.limiters, limiter)
}

func (p *prioritizer) RejectionRate() float64 {
	p.mu.Lock()
	p.mu.Unlock()
	return p.rejectionRate
}

func (p *prioritizer) Calibrate() {
	p.mu.Lock()
	mostOverloaded, in, out, errorValue := p.mostOverloadedLimiter()

	// Update calibrations and compute PI
	integralSum := p.calibrations.add(in, out, errorValue)
	pValue := p.kp * errorValue
	iValue := p.ki * integralSum
	pi := pValue + iValue

	// Instead of adding full PI to current rate, limit how much we can change per calibration
	oldRate := p.rejectionRate
	maxChange := 0.1 // Max 10% change per calibration

	// Scale change by how far PI wants to move
	changeScale := math.Abs(pi) / (math.Abs(pi) + 1) // Asymptotic scaling
	actualChange := maxChange * changeScale * math.Copysign(1, pi)

	newRate := max(0.0, min(1.0, oldRate+actualChange))

	// Apply limits to get actual output
	// newRate := max(0.0, min(1.0, unboundedOutput))
	p.rejectionRate = newRate

	// Compute priority threshold
	newThresh := int32(p.digest.Quantile(newRate))
	p.mu.Unlock()
	oldThresh := p.priorityThreshold.Swap(newThresh)

	if p.logger != nil && p.logger.Enabled(nil, slog.LevelDebug) {
		p.logger.Debug("prioritizer calibration",
			"newRate", fmt.Sprintf("%.2f", newRate),
			"newThresh", newThresh,
			"mostOverloaded", mostOverloaded.name(),
			"errorValue", fmt.Sprintf("%.2f", errorValue),
			"in", in,
			"out", out,
			"blocked", mostOverloaded.Blocked(),
			"pi", fmt.Sprintf("%.2f", pi),
			"newThresh", newThresh,
		)
	}

	if oldThresh != newThresh && p.listener != nil {
		p.listener(PriorityChangedEvent{
			OldPriorityThreshold: uint(oldThresh),
			NewPriorityThreshold: uint(newThresh),
		})
	}
}

func (p *prioritizer) mostOverloadedLimiter() (pidStats, int, int, float64) {
	var maxError float64
	var mostOverloaded pidStats
	var mostOverloadedIn, mostOverloadedOut int
	for _, limiter := range p.limiters {
		// Reset stats
		in, out := limiter.getAndResetStats()
		freeInflight := limiter.Limit() - limiter.Inflight()
		errorValue := computeError(in, out, freeInflight, limiter.Limit())

		if mostOverloaded == nil || errorValue > maxError {
			maxError = errorValue
			mostOverloaded = limiter
			mostOverloadedIn, mostOverloadedOut = in, out
		}
	}
	return mostOverloaded, mostOverloadedIn, mostOverloadedOut, maxError
}

func (p *prioritizer) ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc {
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

func (p *prioritizer) recordPriority(priority int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.digest.Add(float64(priority), 1.0)
}

func (p *prioritizer) threshold() int {
	return int(p.priorityThreshold.Load())
}
