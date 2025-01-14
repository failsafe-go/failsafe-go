package adaptivelimiter

import (
	"fmt"
	"testing"
)

// PriorityDistribution tracks the cumulative distribution of priorities
type PriorityDistribution struct {
	// Ring buffer to track last N priorities
	window     []int
	windowSize int
	currentIdx int

	// Counts for each priority level
	counts [4]int

	// Cumulative distribution (updated when needed)
	cumulative  [4]float64
	needsUpdate bool
}

func NewPriorityDistribution(windowSize int) *PriorityDistribution {
	return &PriorityDistribution{
		window:      make([]int, windowSize),
		windowSize:  windowSize,
		needsUpdate: true,
	}
}

// Add a new priority to the distribution
func (pd *PriorityDistribution) Add(priority int) {
	if priority < 0 || priority > 3 {
		return
	}

	// If window is full, remove the oldest priority
	if pd.window[pd.currentIdx] != 0 {
		oldPriority := pd.window[pd.currentIdx]
		pd.counts[oldPriority]--
	}

	// Add new priority
	pd.window[pd.currentIdx] = priority
	pd.counts[priority]++

	// Move to next position in ring buffer
	pd.currentIdx = (pd.currentIdx + 1) % pd.windowSize
	pd.needsUpdate = true
}

// updateCumulativeDistribution recalculates the cumulative distribution
func (pd *PriorityDistribution) updateCumulativeDistribution() {
	if !pd.needsUpdate {
		return
	}

	total := float64(0)
	for i := range pd.counts {
		total += float64(pd.counts[i])
	}

	// Calculate cumulative distribution
	cumulative := float64(0)
	for i := 0; i < 4; i++ {
		if total > 0 {
			cumulative += float64(pd.counts[i]) / total
		}
		pd.cumulative[i] = cumulative
	}

	pd.needsUpdate = false
}

// GetPriorityThreshold converts a rejection ratio to a priority threshold
func (pd *PriorityDistribution) GetPriorityThreshold(rejectionRatio float64) int {
	pd.updateCumulativeDistribution()

	// Find the highest priority level where cumulative distribution
	// is less than or equal to rejection ratio
	for i := 0; i < 4; i++ {
		if pd.cumulative[i] > rejectionRatio {
			return i
		}
	}

	return 3 // If we can't find a threshold, use highest priority
}

// GetDistribution returns the current priority distribution
func (pd *PriorityDistribution) GetDistribution() []float64 {
	pd.updateCumulativeDistribution()
	distribution := make([]float64, 4)
	copy(distribution, pd.cumulative[:])
	return distribution
}

// Example usage:
func TestFoo(t *testing.T) {
	dist := NewPriorityDistribution(1000)

	// Simulate some traffic pattern
	priorities := []struct {
		priority int
		count    int
	}{
		{0, 400}, // 40% priority 0
		{1, 300}, // 30% priority 1
		{2, 200}, // 20% priority 2
		{3, 100}, // 10% priority 3
	}

	// Add priorities to distribution
	for _, p := range priorities {
		for i := 0; i < p.count; i++ {
			dist.Add(p.priority)
		}
	}

	// Test different rejection ratios
	testRatios := []float64{0.3, 0.5, 0.8, 0.95}
	for _, ratio := range testRatios {
		threshold := dist.GetPriorityThreshold(ratio)
		fmt.Printf("Rejection ratio %.2f -> Priority threshold %d\n", ratio, threshold)
	}

	// Print current distribution
	distribution := dist.GetDistribution()
	fmt.Printf("\nCumulative distribution:\n")
	for i, cum := range distribution {
		fmt.Printf("Priority %d: %.2f\n", i, cum)
	}
}
