package adaptivethrottler

import (
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/priority"
)

// NewPrioritizer returns a new Prioritizer.
func NewPrioritizer() priority.Prioritizer {
	return NewPrioritizerBuilder().Build()
}

// NewPrioritizerBuilder returns a new PrioritizerBuilder.
func NewPrioritizerBuilder() priority.PrioritizerBuilder {
	return &internal.BasePrioritizerConfig[*throttlerStats]{
		Strategy: &throttlerRejectionStrategy{},
	}
}

// Implements priority.RejectionStrategy.
type throttlerRejectionStrategy struct{}

// CombineStats combines throttler stats using a weighted rejection rate, where weights are based on the number of
// executions, to determine a weighted average rejection rate.
func (s *throttlerRejectionStrategy) CombineStats(statsFuncs []func() *throttlerStats) *throttlerStats {
	if len(statsFuncs) == 0 {
		return &throttlerStats{}
	}

	totalExecutions := 0.0
	var totalWeightedRejectionRate float64
	minMaxRejectionRate := 1.0

	for _, statsFn := range statsFuncs {
		stats := statsFn()
		if stats == nil {
			continue
		}

		totalExecutions += stats.executions
		totalWeightedRejectionRate += stats.rejectionRate * stats.executions
		minMaxRejectionRate = min(minMaxRejectionRate, stats.maxRejectionRate)
	}

	var weightedAvgRejectionRate float64
	if totalExecutions > 0 {
		weightedAvgRejectionRate = totalWeightedRejectionRate / totalExecutions
	}

	return &throttlerStats{
		executions:       totalExecutions,
		rejectionRate:    weightedAvgRejectionRate,
		maxRejectionRate: minMaxRejectionRate,
	}
}
