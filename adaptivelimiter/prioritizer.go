package adaptivelimiter

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/failsafe-go/failsafe-go/priority"
)

// Prioritizer computes rejection rates and thresholds for priority limiters based on system overload. When limiters
// become full and start queueing executions, the Prioritizer determines which priority levels should be rejected to
// shed load. Individual executions can be assigned different priority levels, with higher priorities more likely to be
// accepted during overload. A Prioritizer can coordinate across multiple limiters to make rejection decisions based on
// overall system queueing.
//
// In order to operate correctly, a Prioritizer needs to be regularly calibrated, either by calling Calibrate at regular
// intervals, or by using ScheduleCalibrations.
//
// This type is concurrency safe.
type Prioritizer interface {
	// RejectionRate returns the current rate, from 0 to 1, at which executions will be rejected, based on recent
	// queueing levels across registered limiters.
	RejectionRate() float64

	// RejectionThreshold returns the threshold below which executions will be rejected, based on their priority level
	// (0-499). Higher priority executions are more likely to be accepted when the system is overloaded.
	RejectionThreshold() int

	// Calibrate recalculates the RejectionRate and RejectionThreshold based on current queueing levels from registered limiters.
	Calibrate()

	// ScheduleCalibrations runs Calibrate on the interval until the ctx is done or the returned CancelFunc is called.
	ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc

	// Registers a queue stats func for a limiter, to be used for combined rejection threshold calculations.
	register(queueStatsFn queueStatsFunc)
}

// PrioritizerBuilder builds Prioritizer instances.
//
// This type is not concurrency safe.
type PrioritizerBuilder interface {
	// WithLevelTracker configures a level tracker to use with the prioritizer. The level tracker can be shared across
	// different policy instances and types.
	WithLevelTracker(levelTracker priority.LevelTracker) PrioritizerBuilder

	// WithLogger configures a logger which provides debug logging of calibrations.
	WithLogger(logger *slog.Logger) PrioritizerBuilder

	// OnThresholdChanged configures a listener to be called with the rejection threshold for rejection changes.
	OnThresholdChanged(listener func(event ThresholdChangedEvent)) PrioritizerBuilder

	// Build returns a new Prioritizer using the builder's configuration.
	Build() Prioritizer
}

// ThresholdChangedEvent indicates a Prioritizer's rejection threshold has changed.
type ThresholdChangedEvent struct {
	OldThreshold uint
	NewThreshold uint
}

type prioritizerConfig struct {
	logger             *slog.Logger
	levelTracker       priority.LevelTracker
	onThresholdChanged func(event ThresholdChangedEvent)
}

var _ PrioritizerBuilder = &prioritizerConfig{}

// NewPrioritizer returns a new Prioritizer.
func NewPrioritizer() Prioritizer {
	return NewPrioritizerBuilder().Build()
}

// NewPrioritizerBuilder returns a new PrioritizerBuilder.
func NewPrioritizerBuilder() PrioritizerBuilder {
	return &prioritizerConfig{}
}

func (c *prioritizerConfig) WithLevelTracker(levelTracker priority.LevelTracker) PrioritizerBuilder {
	c.levelTracker = levelTracker
	return c
}

func (c *prioritizerConfig) WithLogger(logger *slog.Logger) PrioritizerBuilder {
	c.logger = logger
	return c
}

func (c *prioritizerConfig) OnThresholdChanged(listener func(event ThresholdChangedEvent)) PrioritizerBuilder {
	c.onThresholdChanged = listener
	return c
}

func (c *prioritizerConfig) Build() Prioritizer {
	pCopy := *c
	if pCopy.levelTracker == nil {
		pCopy.levelTracker = priority.NewLevelTracker()
	}
	return &prioritizer{
		prioritizerConfig: &pCopy, // TODO copy base fields
	}
}

// Define limiter operations that don't depend on a result type.
type queueStatsFunc func() (limit, queued, rejectionThreshold, maxQueue int)

type prioritizer struct {
	*prioritizerConfig

	// Mutable state
	rejectionThreshold atomic.Int32
	mu                 sync.Mutex
	statsFuncs         []queueStatsFunc // Guarded by mu
	rejectionRate      float64          // Guarded by mu
}

func (p *prioritizer) register(statsFunc queueStatsFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statsFuncs = append(p.statsFuncs, statsFunc)
}

func (p *prioritizer) RejectionRate() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rejectionRate
}

func (p *prioritizer) RejectionThreshold() int {
	return int(p.rejectionThreshold.Load())
}

func (p *prioritizer) Calibrate() {
	p.mu.Lock()

	// Compute queue stats across all registered stats funcs
	var totalLimit, totalQueued, totalRejectionThresh, totalMaxQueue int
	for _, statsFunc := range p.statsFuncs {
		limit, queued, rejectionThresh, maxQueue := statsFunc()
		totalLimit += limit
		totalQueued += queued
		totalRejectionThresh += rejectionThresh
		totalMaxQueue += maxQueue
	}

	// Update rejection rate and rejection threshold
	newRate := computeRejectionRate(totalQueued, totalRejectionThresh, totalMaxQueue)
	p.rejectionRate = newRate
	var newThresh int32
	if newRate > 0 {
		newThresh = int32(p.levelTracker.GetLevel(newRate))
	}
	p.mu.Unlock()
	oldThresh := p.rejectionThreshold.Swap(newThresh)

	if p.logger != nil && p.logger.Enabled(nil, slog.LevelDebug) {
		p.logger.Debug("prioritizer calibration",
			"newRate", fmt.Sprintf("%.2f", newRate),
			"newThresh", newThresh,
			"limit", totalLimit,
			"queued", totalQueued)
	}

	if oldThresh != newThresh && p.onThresholdChanged != nil {
		p.onThresholdChanged(ThresholdChangedEvent{
			OldThreshold: uint(oldThresh),
			NewThreshold: uint(newThresh),
		})
	}
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
