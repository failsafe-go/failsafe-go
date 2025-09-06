package adaptivethrottler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestThrottlerRejectionStrategy_CombineStats(t *testing.T) {
	strategy := &throttlerRejectionStrategy{}

	t.Run("with empty stats", func(t *testing.T) {
		result := strategy.CombineStats([]func() *throttlerStats{})

		assert.Equal(t, 0.0, result.executions)
		assert.Equal(t, 0.0, result.rejectionRate)
		assert.Equal(t, 0.0, result.maxRejectionRate)
	})

	t.Run("with single throttler", func(t *testing.T) {
		statsFuncs := []func() *throttlerStats{
			func() *throttlerStats {
				return &throttlerStats{
					executions:       100,
					rejectionRate:    0.2,
					maxRejectionRate: 0.8,
				}
			},
		}

		result := strategy.CombineStats(statsFuncs)

		assert.Equal(t, 100.0, result.executions)
		assert.Equal(t, 0.2, result.rejectionRate)
		assert.Equal(t, 0.8, result.maxRejectionRate)
	})

	t.Run("with two throttlers with equal executions", func(t *testing.T) {
		statsFuncs := []func() *throttlerStats{
			func() *throttlerStats {
				return &throttlerStats{
					executions:       100,
					rejectionRate:    0.2,
					maxRejectionRate: 0.8,
				}
			},
			func() *throttlerStats {
				return &throttlerStats{
					executions:       100,
					rejectionRate:    0.4,
					maxRejectionRate: 0.9,
				}
			},
		}

		result := strategy.CombineStats(statsFuncs)

		assert.Equal(t, 200.0, result.executions)
		assert.Equal(t, 0.3, result.rejectionRate)
		assert.Equal(t, 0.8, result.maxRejectionRate)
	})

	t.Run("with two throttlers with different execution volumes", func(t *testing.T) {
		statsFuncs := []func() *throttlerStats{
			func() *throttlerStats {
				return &throttlerStats{
					executions:       150.0,
					rejectionRate:    0.6,
					maxRejectionRate: 0.9,
				}
			},
			func() *throttlerStats {
				return &throttlerStats{
					executions:       50.0,
					rejectionRate:    0.1,
					maxRejectionRate: 0.8,
				}
			},
		}

		result := strategy.CombineStats(statsFuncs)

		assert.Equal(t, 200.0, result.executions)
		assert.Equal(t, 0.475, result.rejectionRate)
		assert.Equal(t, 0.8, result.maxRejectionRate)
	})

	t.Run("with a throttler with zero executions", func(t *testing.T) {
		statsFuncs := []func() *throttlerStats{
			func() *throttlerStats {
				return &throttlerStats{
					executions:       100.0,
					rejectionRate:    0.3,
					maxRejectionRate: 0.9,
				}
			},
			func() *throttlerStats {
				return &throttlerStats{
					executions:       0.0,
					rejectionRate:    0.8,
					maxRejectionRate: 0.7,
				}
			},
		}

		result := strategy.CombineStats(statsFuncs)

		assert.Equal(t, 100.0, result.executions)
		assert.Equal(t, 0.3, result.rejectionRate)
		assert.Equal(t, 0.7, result.maxRejectionRate)
	})

	t.Run("with nil stats are ignored", func(t *testing.T) {
		statsFuncs := []func() *throttlerStats{
			func() *throttlerStats {
				return nil
			},
			func() *throttlerStats {
				return &throttlerStats{
					executions:       100.0,
					rejectionRate:    0.4,
					maxRejectionRate: 0.9,
				}
			},
		}

		result := strategy.CombineStats(statsFuncs)

		assert.Equal(t, 100.0, result.executions)
		assert.Equal(t, 0.4, result.rejectionRate)
		assert.Equal(t, 0.9, result.maxRejectionRate)
	})
}
