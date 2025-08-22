package util

// Ewma is an exponentially weighted moving average.
//
// This type is not concurrency safe.
type Ewma struct {
	warmupSamples   uint8
	smoothingFactor float64

	// Mutable state
	count uint8
	value float64
	sum   float64
}

// NewEwma creates a new Ewma for the windowSize and warmupSamples. windowSize controls how many samples are effectively
// stored in the Ewma before they decay out. warmupSamples controls how many samples must be recorded before decay
// begins.
func NewEwma(windowSize uint, warmupSamples uint8) *Ewma {
	return &Ewma{
		warmupSamples:   warmupSamples,
		smoothingFactor: 2 / (float64(windowSize) + 1),
	}
}

// Add adds a value to the series and updates the moving average. Add decays the Ewma value via (oldValue * (1 -
// smoothingFactor)) + (newValue * smoothingFactor)
func (e *Ewma) Add(newValue float64) float64 {
	switch {
	case e.count < e.warmupSamples:
		e.count++
		e.sum += newValue
		e.value = e.sum / float64(e.count)
	default:
		e.value = Smooth(e.value, newValue, e.smoothingFactor)
	}
	return e.value
}

// Value gets the current value of the moving average.
func (e *Ewma) Value() float64 {
	return e.value
}

// Reset resets the value of the moving average and requires a new warmup if one was configured.
func (e *Ewma) Reset() {
	e.count = 0
	e.value = 0
	e.sum = 0
}
