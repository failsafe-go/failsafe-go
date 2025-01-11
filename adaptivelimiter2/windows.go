package adaptivelimiter2

import (
	"math"
	"time"

	"github.com/influxdata/tdigest"
)

type td struct {
	minRTT time.Duration
	size   uint
	*tdigest.TDigest
}

func (td *td) add(rtt time.Duration) {
	td.Add(float64(rtt), 1)
	td.minRTT = min(td.minRTT, rtt)
	td.size++
}

func (td *td) reset() {
	td.Reset()
	td.minRTT = 0
	td.size = 0
}

func newRollingSum(capacity uint) *rollingSum {
	return &rollingSum{samples: make([]float64, capacity)}
}

type rollingSum struct {
	// For variation and covariance
	samples []float64
	size    int
	index   int

	// Rolling sum fields
	sumY       float64 // Y values are the samples
	sumSquares float64

	// Rolling regression fields
	sumX  float64 // X values are the sample indexes
	sumXX float64
	sumXY float64
}

// add adds the value to the window if it's non-zero, updates the sum and sumSquares, and returns the mean, variance,
// and coefficient of variation for the samples in the window, along with the old value, and whether the window was full.
// Returns NaN for the cv if there are < 2 samples, the variance is < 0, or the mean is 0.
func (r *rollingSum) addToSum(value float64) (oldValue float64, full bool) {
	if value != 0 {
		if r.size == len(r.samples) {
			full = true

			// Remove oldest value
			oldValue = r.samples[r.index]
			r.sumY -= oldValue
			r.sumSquares -= oldValue * oldValue

			// Shift all values left (via telescoping series)
			r.sumXY = r.sumXY - r.sumY

			// Add new value at the end
			r.sumXY += float64(len(r.samples)-1) * value
		} else {
			r.sumXY += float64(r.size) * value
			r.size++
		}

		// Add new value
		r.samples[r.index] = value

		// Update rolling computations
		r.sumY += value
		r.sumSquares += value * value

		// Move index forward
		r.index = (r.index + 1) % len(r.samples)
	}

	return oldValue, full
}

// Calculates the coefficient of variation (relative variance) for the sum
func (r *rollingSum) calculateCV() (mean, variance, cv float64) {
	if r.size < 2 {
		return math.NaN(), math.NaN(), math.NaN()
	}

	mean = r.sumY / float64(r.size)
	variance = (r.sumSquares / float64(r.size)) - (mean * mean)
	if variance < 0 || mean == 0 {
		return math.NaN(), math.NaN(), math.NaN()
	}

	cv = math.Sqrt(variance) / mean
	return mean, variance, cv
}

func (r *rollingSum) calculateSlope() float64 {
	if r.size < 2 {
		return math.NaN()
	}

	// Calculate slope using least squares
	n := float64(r.size)
	sumX := n * (n - 1) / 2
	sumXSquared := n * (n - 1) * (2*n - 1) / 6
	return (n*r.sumXY - sumX*r.sumY) / (n*sumXSquared - sumX*sumX)
}

type variationWindow struct {
	*rollingSum
}

func newVariationWindow(capacity uint) *variationWindow {
	return &variationWindow{rollingSum: newRollingSum(capacity)}
}

// add adds the value to the window if it's non-zero and returns the coefficient of variation for the samples in the window.
// Returns 1 if there are < 2 samples, the variance is < 0, or the mean is 0.
func (w *variationWindow) add(value float64) float64 {
	w.addToSum(value)
	_, _, cv := w.calculateCV()
	if math.IsNaN(cv) {
		return 1.0
	}
	return cv
}

type correlationWindow struct {
	warmupSamples uint8
	xSamples      *rollingSum
	ySamples      *rollingSum
	corrSumXY     float64
}

func newCorrelationWindow(capacity uint, warmupSamples uint8) *correlationWindow {
	return &correlationWindow{
		warmupSamples: warmupSamples,
		xSamples:      newRollingSum(capacity),
		ySamples:      newRollingSum(capacity),
	}
}

// add adds the values to the window and returns the current correlation coefficient.
// Returns a value between 0 and 1 when a correlation between increasing x and y values is present.
// Returns a value between -1 and 0 when a correlation between increasing x and decreasing y values is present.
// Returns 0 if < warmup or low variation (< .01)
func (w *correlationWindow) add(x, y float64) (correlation, cvX, cvY float64) {
	oldX, full := w.xSamples.addToSum(x)
	oldY, _ := w.ySamples.addToSum(y)
	meanX, varX, cvX := w.xSamples.calculateCV()
	meanY, varY, cvY := w.ySamples.calculateCV()

	if full {
		// Remove old value
		w.corrSumXY -= oldX * oldY
	}

	// Add new value
	w.corrSumXY += x * y

	if math.IsNaN(cvX) || math.IsNaN(cvY) {
		return 0, 0, 0
	}

	// Ignore weak correlations
	if w.xSamples.size < int(w.warmupSamples) {
		return 0, 0, 0
	}

	// Ignore measurements that vary by less than 1%
	minCV := 0.01
	if cvX < minCV || cvY < minCV {
		return 0, cvX, cvY
	}

	covariance := (w.corrSumXY / float64(w.xSamples.size)) - (meanX * meanY)
	correlation = covariance / (math.Sqrt(varX) * math.Sqrt(varY))

	return correlation, cvX, cvY
}
