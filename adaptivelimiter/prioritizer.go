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

// Prioritizer computes a rejection rate and priority threshold for one or more priority limiters, which can be used to
// determine whether to accept or reject an execution.
type Prioritizer interface {
	// RejectionRate returns the current rate, from 0 to 1, at which the limiter will reject requests, based on recent
	// execution times.
	RejectionRate() float64

	// Calibrate calibrates the RejectionRate based on recent execution times from registered limiters.
	Calibrate()

	// ScheduleCalibrations runs calibration on the interval until the ctx is done or the returned CancelFunc is called.
	ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc

	register(limiter limiterStats)
	recordPriority(priority int)
	threshold() int
}

// PrioritizerBuilder builds Prioritizer instances.
//
// This type is not concurrency safe.
type PrioritizerBuilder interface {
	// OnThresholdChanged configures a listener to be called with the priority threshold for rejection changes.
	OnThresholdChanged(listener func(event ThresholdChangedEvent)) PrioritizerBuilder

	// WithLogger configures a logger which provides debug logging of calibrations.
	WithLogger(logger *slog.Logger) PrioritizerBuilder

	// Build returns a new Prioritizer using the builder's configuration.
	Build() Prioritizer
}

// ThresholdChangedEvent indicates an Prioritizer's priority threshold has changed.
type ThresholdChangedEvent struct {
	OldPriorityThreshold uint
	NewPriorityThreshold uint
}

type prioritizerConfig struct {
	logger   *slog.Logger
	listener func(event ThresholdChangedEvent)
}

var _ PrioritizerBuilder = &prioritizerConfig{}

func NewPrioritizer() Prioritizer {
	return NewPrioritizerBuilder().Build()
}

// NewPrioritizerBuilder returns a PrioritizerBuilder.
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
	getAndResetStats() (in, out, limit, inflight, queued, maxQueue int)
}

type prioritizer struct {
	*prioritizerConfig

	// Mutable state
	priorityThreshold atomic.Int32
	mu                sync.Mutex
	limiters          []limiterStats   // Guarded by mu
	digest            *tdigest.TDigest // Guarded by mu
	rejectionRate     float64          // Guarded by mu
}

func (r *prioritizer) register(limiter limiterStats) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limiters = append(r.limiters, limiter)
}

func (r *prioritizer) RejectionRate() float64 {
	r.mu.Lock()
	r.mu.Unlock()
	return r.rejectionRate
}

func (r *prioritizer) Calibrate() {
	r.mu.Lock()

	// Compute queue stats across all registered limiters
	var totalIn, totalOut, totalLimit, totalQueued, totalFreeInflight, totalMaxQueue int
	for _, limiter := range r.limiters {
		in, out, limit, inflight, queued, maxQueue := limiter.getAndResetStats()
		totalIn += in
		totalOut += out
		totalFreeInflight += limit - inflight
		totalLimit += limit
		totalQueued += queued
		totalMaxQueue += maxQueue
	}

	// Update rejection rate and priority threshold
	errorValue := computeError(totalIn, totalOut, totalFreeInflight, totalQueued, totalMaxQueue)
	newRate := computeRejectionRate(totalQueued, totalMaxQueue)
	r.rejectionRate = newRate
	var newThresh int32
	if newRate > 0 {
		newThresh = int32(r.digest.Quantile(newRate))
	}
	r.mu.Unlock()
	oldThresh := r.priorityThreshold.Swap(newThresh)

	if r.logger != nil && r.logger.Enabled(nil, slog.LevelDebug) {
		r.logger.Debug("prioritizer calibration",
			"newRate", fmt.Sprintf("%.2f", newRate),
			"newThresh", newThresh,
			"in", totalIn,
			"out", totalOut,
			"limit", totalLimit,
			"blocked", totalQueued,
			"error", fmt.Sprintf("%.2f", errorValue),
		)
	}

	if oldThresh != newThresh && r.listener != nil {
		r.listener(ThresholdChangedEvent{
			OldPriorityThreshold: uint(oldThresh),
			NewPriorityThreshold: uint(newThresh),
		})
	}
}

// Computes an error for the queue stats. A positive error indicates overloaded. A negative error indicates underloaded.
func computeError(in, out, freeInflight int, queueSize, maxQueueSize int) float64 {
	load := float64(queueSize + (in - out))
	capacity := float64(maxQueueSize + freeInflight)
	excessLoad := load - capacity
	return excessLoad / capacity
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

func (r *prioritizer) threshold() int {
	return int(r.priorityThreshold.Load())
}
