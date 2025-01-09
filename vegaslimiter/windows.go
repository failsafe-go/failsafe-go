package vegaslimiter

import (
	"math"
	"time"

	"github.com/influxdata/tdigest"
)

// type rttWindow struct {
// 	minRTT time.Duration
// 	rttSum time.Duration
// 	size   uint
// }
//
// func newRTTWindow() *rttWindow {
// 	return &rttWindow{
// 		minRTT: 24 * time.Hour,
// 	}
// }
//
// func (w *rttWindow) add(rtt time.Duration) {
// 	w.minRTT = min(w.minRTT, rtt)
// 	w.rttSum += rtt
// 	w.size++
// }
//
// func (w *rttWindow) average() time.Duration {
// 	if w.size == 0 {
// 		return 0
// 	}
// 	return w.rttSum / time.Duration(w.size)
// }

type rttWindow struct {
	minRTT time.Duration
	rttSum time.Duration
	size   uint
}

func newRTTWindow() *rttWindow {
	return &rttWindow{
		minRTT: 24 * time.Hour,
	}
}

func (w *rttWindow) add(rtt time.Duration) {
	w.minRTT = min(w.minRTT, rtt)
	w.rttSum += rtt
	w.size++
}

func (w *rttWindow) average() time.Duration {
	if w.size == 0 {
		return 0
	}
	return w.rttSum / time.Duration(w.size)
}

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

type rollingSum struct {
	samples    []float64
	size       int
	index      int
	sum        float64
	sumSquares float64
}

// add adds the value to the window if it's non-zero, updates the sum and sumSquares, and returns the mean, variance,
// and coefficient of variation for the samples in the window, along with the old value, and whether the window was full.
// Returns NaN for the cv if there are < 2 samples, the variance is < 0, or the mean is 0.
func (w *rollingSum) addToSum(value float64) (mean, variance, cv, oldValue float64, full bool) {
	if value != 0 {
		if w.size < len(w.samples) {
			w.size++
		} else {
			// Remove old value
			oldValue = w.samples[w.index]
			w.sum -= oldValue
			w.sumSquares -= oldValue * oldValue
			full = true
		}

		// Add new value
		w.samples[w.index] = value
		w.sum += value
		w.sumSquares += value * value

		w.index = (w.index + 1) % len(w.samples)
	}

	// Require at least 2 values to return a result
	if w.size < 2 {
		return math.NaN(), math.NaN(), math.NaN(), oldValue, full
	}

	mean = w.sum / float64(w.size)
	variance = (w.sumSquares / float64(w.size)) - (mean * mean)
	if variance < 0 || mean == 0 {
		return math.NaN(), math.NaN(), math.NaN(), oldValue, full
	}

	// Calculate coefficient of variation (relative variance), which gives us variance as a percentage of the mean
	cv = math.Sqrt(variance) / mean
	return mean, variance, cv, oldValue, full
}

type variationWindow struct {
	*rollingSum
}

func newVariationWindow(capacity uint) *variationWindow {
	return &variationWindow{
		rollingSum: &rollingSum{samples: make([]float64, capacity)},
	}
}

// add adds the value to the window if it's non-zero and returns the coefficient of variation for the samples in the window.
// Returns 1 if there are < 2 samples, the variance is < 0, or the mean is 0.
func (w *variationWindow) add(value float64) float64 {
	_, _, cv, _, _ := w.addToSum(value)
	if math.IsNaN(cv) {
		return 1.0
	}
	return cv
}

type correlationWindow struct {
	warmupSamples uint8
	xSamples      *rollingSum
	ySamples      *rollingSum
	sumXY         float64
}

func newCorrelationWindow(capacity uint, warmupSamples uint8) *correlationWindow {
	return &correlationWindow{
		warmupSamples: warmupSamples,
		xSamples:      &rollingSum{samples: make([]float64, capacity)},
		ySamples:      &rollingSum{samples: make([]float64, capacity)},
	}
}

// add adds the values to the window and returns the current correlation coefficient.
// Returns a value between 0 and 1 when a correlation between increasing x and y values is present.
// Returns a value between -1 and 0 when a correlation between increasing x and decreasing y values is present.
// Returns 0 if < warmup or low variation (< .01)
func (w *correlationWindow) add(x, y float64) (correlation, cvX, cvY float64) {
	meanX, varX, cvX, oldX, full := w.xSamples.addToSum(x)
	meanY, varY, cvY, oldY, _ := w.ySamples.addToSum(y)
	size := w.xSamples.size

	if full {
		// Remove old value
		w.sumXY -= oldX * oldY
	}

	// Add new value
	w.sumXY += x * y

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

	covariance := (w.sumXY / float64(size)) - (meanX * meanY)
	correlation = covariance / (math.Sqrt(varX) * math.Sqrt(varY))

	// fmt.Printf("Correlation calculation: ")
	// fmt.Printf("  x=%.2f y=%.2f ", x, y)
	// fmt.Printf("  meanX=%.2f meanY=%.2f ", meanX, meanY)
	// fmt.Printf("  varX=%.2f varY=%.2f ", varX, varY)
	// fmt.Printf("  stdX=%.2f stdY=%.2f ", math.Sqrt(varX), math.Sqrt(varY))
	// fmt.Printf("  sumXY=%.2f ", w.sumXY)
	// fmt.Printf("  size=%d ", size)
	// fmt.Printf("  covariance=%.2f ", covariance)
	// fmt.Printf("  correlation=%.2f\n", correlation)

	return correlation, cvX, cvY
}
