package adaptivelimiter

import (
	priorityInternal "github.com/failsafe-go/failsafe-go/internal/priority"
	"github.com/failsafe-go/failsafe-go/priority"
)

// NewPrioritizer returns a new Prioritizer.
func NewPrioritizer() priority.Prioritizer {
	return NewPrioritizerBuilder().Build()
}

// NewPrioritizerBuilder returns a new PrioritizerBuilder.
func NewPrioritizerBuilder() PrioritizerBuilder {
	return &prioritizerConfig{
		BasePrioritizerConfig: &priorityInternal.BasePrioritizerConfig[*queueStats]{
			Strategy: &queueRejectionStrategy{},
		},
	}
}

// PrioritizerBuilder builds Prioritizer instances.
//
// This type is not concurrency safe.
type PrioritizerBuilder interface {
	priority.PrioritizerBuilder

	// WithUsageTracker configures a usage tracker to use with the prioritizer. The usage tracker tracks usage as total
	// execution duration, to enforce fairness when rejecting executions.
	WithUsageTracker(usageTracker priority.UsageTracker) PrioritizerBuilder
}

// BasePrioritizerConfig provides a base for implementing a PrioritizerBuilder.
type prioritizerConfig struct {
	*priorityInternal.BasePrioritizerConfig[*queueStats]
}

func (c *prioritizerConfig) WithUsageTracker(usageTracker priority.UsageTracker) PrioritizerBuilder {
	c.UsageTracker = usageTracker
	return c
}

// Implements priority.RejectionStrategy.
type queueRejectionStrategy struct{}

func (s *queueRejectionStrategy) CombineStats(statsFuncs []func() *queueStats) *queueStats {
	var result queueStats
	for _, statsFn := range statsFuncs {
		stats := statsFn()
		result.limit += stats.limit
		result.queued += stats.queued
		result.rejectionThreshold += stats.rejectionThreshold
		result.maxQueue += stats.maxQueue
	}
	return &result
}
