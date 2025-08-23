package adaptivelimiter

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/influxdata/tdigest"
)

// Prioritizer computes a rejection rate and rejection threshold for one or more priority limiters, which can be used to
// determine whether to accept or reject an execution. Individual requests through the same limiter can be prioritized
// differently. A Prioritizer can also be used with multiple limiters, to determine rejection thresholds based on the
// amount of queueing across limiters.
//
// In order to operate correctly, a Prioritizer needs to be regularly calibrated, either by calling Calibrate at regular
// intervals, or by using ScheduleCalibrations.
//
// This type is concurrency safe.
type Prioritizer interface {
	// RejectionRate returns the current rate, from 0 to 1, at which the limiter will reject requests, based on recent
	// execution times.
	RejectionRate() float64

	// RejectionThreshold is the threshold below which requests will be rejected, based on their level, from 0 to 499.
	RejectionThreshold() int

	// Calibrate calibrates the RejectionRate based on recent execution times from registered limiters.
	Calibrate()

	// ScheduleCalibrations runs Calibrate on the interval until the ctx is done or the returned CancelFunc is called.
	ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc

	register(limiter limiterStats)
	recordPriority(priority int)
}

// PrioritizerBuilder builds Prioritizer instances.
//
// This type is not concurrency safe.
type PrioritizerBuilder interface {
	// OnThresholdChanged configures a listener to be called with the rejection threshold for rejection changes.
	OnThresholdChanged(listener func(event ThresholdChangedEvent)) PrioritizerBuilder

	// WithLogger configures a logger which provides debug logging of calibrations.
	WithLogger(logger *slog.Logger) PrioritizerBuilder

	// Build returns a new Prioritizer using the builder's configuration.
	Build() Prioritizer
}

// ThresholdChangedEvent indicates a Prioritizer's rejection threshold has changed.
type ThresholdChangedEvent struct {
	OldThreshold uint
	NewThreshold uint
}

type prioritizerConfig struct {
	logger   *slog.Logger
	listener func(event ThresholdChangedEvent)
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

func (c *prioritizerConfig) WithLogger(logger *slog.Logger) PrioritizerBuilder {
	c.logger = logger
	return c
}

func (c *prioritizerConfig) OnThresholdChanged(listener func(event ThresholdChangedEvent)) PrioritizerBuilder {
	c.listener = listener
	return c
}

func (c *prioritizerConfig) Build() Prioritizer {
	pCopy := *c
	return &prioritizer{
		prioritizerConfig: &pCopy, // TODO copy base fields
		digest:            tdigest.NewWithCompression(100),
	}
}

// Define limiter operations that don't depend on a result type.
type limiterStats interface {
	queueStats() (limit, queued, rejectionThreshold, maxQueue int)
}

type prioritizer struct {
	*prioritizerConfig

	// Mutable state
	rejectionThreshold atomic.Int32
	mu                 sync.Mutex
	limiters           []limiterStats   // Guarded by mu
	digest             *tdigest.TDigest // Guarded by mu
	rejectionRate      float64          // Guarded by mu
}

func (r *prioritizer) register(limiter limiterStats) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limiters = append(r.limiters, limiter)
}

func (r *prioritizer) RejectionRate() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rejectionRate
}

func (r *prioritizer) RejectionThreshold() int {
	return int(r.rejectionThreshold.Load())
}

func (r *prioritizer) Calibrate() {
	r.mu.Lock()

	// Compute queue stats across all registered limiters
	var totalLimit, totalQueued, totalRejectionThresh, totalMaxQueue int
	for _, limiter := range r.limiters {
		limit, queued, rejectionThresh, maxQueue := limiter.queueStats()
		totalLimit += limit
		totalQueued += queued
		totalRejectionThresh += rejectionThresh
		totalMaxQueue += maxQueue
	}

	// Update rejection rate and rejection threshold
	newRate := computeRejectionRate(totalQueued, totalRejectionThresh, totalMaxQueue)
	r.rejectionRate = newRate
	var newThresh int32
	if newRate > 0 {
		newThresh = int32(r.digest.Quantile(newRate))
	}
	r.mu.Unlock()
	oldThresh := r.rejectionThreshold.Swap(newThresh)

	if r.logger != nil && r.logger.Enabled(nil, slog.LevelDebug) {
		r.logger.Debug("prioritizer calibration",
			"newRate", fmt.Sprintf("%.2f", newRate),
			"newThresh", newThresh,
			"limit", totalLimit,
			"queued", totalQueued)
	}

	if oldThresh != newThresh && r.listener != nil {
		r.listener(ThresholdChangedEvent{
			OldThreshold: uint(oldThresh),
			NewThreshold: uint(newThresh),
		})
	}
}

func (r *prioritizer) ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc {
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
				r.Calibrate()
			}
		}
	}()

	return func() {
		close(done)
	}
}

func (r *prioritizer) recordPriority(priority int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.digest.Add(float64(priority), 1.0)
}
