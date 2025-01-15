package adaptivelimiter

import (
	"context"
	"fmt"
	"log/slog"
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

	register(limiter rttEstimator)
	recordPriority(priority int)
	threshold() int
}

// PrioritizerBuilder builds Prioritizer instances.
//
// This type is not concurrency safe.
type PrioritizerBuilder interface {
	// WithLogger configures a logger which provides debug logging of calibrations.
	WithLogger(logger *slog.Logger) PrioritizerBuilder

	// Build returns a new Prioritizer using the builder's configuration.
	Build() Prioritizer
}

type prioritizerConfig struct {
	logger             *slog.Logger
	rejectionThreshold time.Duration
	maxExecutionTime   time.Duration
}

var _ PrioritizerBuilder = &prioritizerConfig{}

func NewPrioritizer(rejectionThreshold time.Duration, maxExecutionTime time.Duration) Prioritizer {
	return NewPrioritizerBuilder(rejectionThreshold, maxExecutionTime).Build()
}

// NewPrioritizerBuilder returns a PrioritizerBuilder.
func NewPrioritizerBuilder(rejectionThreshold time.Duration, maxExecutionTime time.Duration) PrioritizerBuilder {
	return &prioritizerConfig{
		rejectionThreshold: rejectionThreshold,
		maxExecutionTime:   maxExecutionTime,
	}
}

func (c *prioritizerConfig) WithLogger(logger *slog.Logger) PrioritizerBuilder {
	c.logger = logger
	return c
}

func (c *prioritizerConfig) Build() Prioritizer {
	pCopy := *c
	return &prioritizer{
		prioritizerConfig: &pCopy, // TODO copy base fields
		digest:            tdigest.NewWithCompression(100),
	}
}

type rttEstimator interface {
	averageRTT() float64
	estimateRTT() time.Duration
}

type prioritizer struct {
	*prioritizerConfig

	// Mutable state
	priorityThreshold atomic.Int32
	mu                sync.Mutex
	limiters          []rttEstimator   // Guarded by mu
	digest            *tdigest.TDigest // Guarded by mu
	rejectionRate     float64          // Guarded by mu
}

func (p *prioritizer) register(limiter rttEstimator) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.limiters = append(p.limiters, limiter)
}

func (p *prioritizer) RejectionRate() float64 {
	p.mu.Lock()
	p.mu.Unlock()
	return p.rejectionRate
}

// Calibrate computes a rejection rate and priority threshold based on the rtt from the most overloaded limiter.
func (p *prioritizer) Calibrate() {
	p.mu.Lock()
	mostOverloaded := p.mostOverloadedLimiter()
	if mostOverloaded == nil {
		p.mu.Unlock()
		return
	}

	// Compute rejection rate
	rtt := mostOverloaded.estimateRTT()
	rejectionRate := max(0, computeRejectionRate(rtt, p.rejectionThreshold, p.maxExecutionTime))
	p.rejectionRate = rejectionRate

	// Compute priority threshold
	var thresh int
	if rejectionRate != 0 {
		thresh = int(p.digest.Quantile(rejectionRate))
	}
	p.mu.Unlock()
	p.priorityThreshold.Store(int32(thresh))

	if p.logger != nil && p.logger.Enabled(nil, slog.LevelDebug) {
		p.logger.Debug("prioritizer calibration",
			"rtt", rtt.Milliseconds(),
			"rejectionRate", fmt.Sprintf("%.2f", rejectionRate),
			"priorityThresh", thresh,
		)
	}
}

func (p *prioritizer) mostOverloadedLimiter() rttEstimator {
	var maxRTT float64
	var mostOverloaded rttEstimator
	for _, limiter := range p.limiters {
		rtt := limiter.averageRTT()
		if mostOverloaded == nil || rtt > maxRTT {
			mostOverloaded = limiter
			maxRTT = rtt
		}
	}
	return mostOverloaded
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
