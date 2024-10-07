package adaptivelimiter

// MovingAverage is not concurrency safe.
type MovingAverage interface {
	// Add adds a value to the series and updates the moving average.
	Add(float64) float64
	// Value gets the current value of the moving average.
	Value() float64
	// Set sets a value for the moving average.
	Set(float64)
}

// NewEWMA creates a new exponentially weighted moving average for the windowSize and warmupSamples.
// windowSize controls how many samples are effectively stored in the EWMA before they decay out.
// warmupSamples controls how many samples must be recorded before decay begins.
func NewEWMA(windowSize uint, warmupSamples uint8) MovingAverage {
	return &ewma{
		warmupSamples:   warmupSamples,
		smoothingFactor: 2 / (float64(windowSize) + 1),
	}
}

type ewma struct {
	warmupSamples   uint8
	count           uint8
	smoothingFactor float64
	value           float64
	sum             float64
}

// Add decays via (current sample * smoothingFactor) + (previous value * (1 - smoothingFactor)
func (e *ewma) Add(value float64) float64 {
	switch {
	case e.count < e.warmupSamples:
		e.count++
		e.sum += value
		e.value = e.sum / float64(e.count)
	default:
		e.value = smooth(e.value, value, e.smoothingFactor)
	}
	return e.value
}

func (e *ewma) Value() float64 {
	return e.value
}

func (e *ewma) Set(value float64) {
	e.value = value
	// Skip any incomplete warmup
	if e.count < e.warmupSamples {
		e.count = e.warmupSamples
	}
}

func smooth(oldValue, newValue, factor float64) float64 {
	// Decrease by some portion of the oldValue, and increase by some portion of the newValue
	return oldValue*(1-factor) + newValue*factor
}
