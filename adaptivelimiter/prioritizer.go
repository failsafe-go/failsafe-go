package adaptivelimiter

import (
	"github.com/failsafe-go/failsafe-go/priority"
)

// NewPrioritizer returns a new Prioritizer.
func NewPrioritizer() priority.Prioritizer {
	return NewPrioritizerBuilder().Build()
}

// NewPrioritizerBuilder returns a new PrioritizerBuilder.
func NewPrioritizerBuilder() priority.PrioritizerBuilder {
	return &priority.BasePrioritizerConfig[*queueStats]{
		Strategy: &queueRejectionStrategy{},
	}
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
