package adaptivelimiter

import (
	"math"
	"time"
)

type rttWindow struct {
	minRTT time.Duration
	rttSum time.Duration
	size   uint
}

func newRTTWindow() *rttWindow {
	return &rttWindow{
		minRTT: math.MaxInt64,
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

type variationWindow struct {
	samples    []float64
	size       int
	index      int
	sum        float64
	sumSquares float64
}

func newVariationWindow(capacity int) *variationWindow {
	return &variationWindow{
		samples: make([]float64, capacity),
	}
}

// add adds the value to the window if it's non-zero and returns the coefficient of variation (relative standard
// deviation) for the samples in the window.
// Returns 1 if there are < 2 samples, the variance is < 0, or the mean is 0.
func (w *variationWindow) add(value float64) float64 {
	if value != 0 {
		if w.size < len(w.samples) {
			w.size++
		} else {
			// Remove old value
			old := w.samples[w.index]
			w.sum -= old
			w.sumSquares -= old * old
		}

		// Add new value
		w.samples[w.index] = value
		w.sum += value
		w.sumSquares += value * value

		w.index = (w.index + 1) % len(w.samples)
	}

	// Require at least 2 values
	if w.size < 2 {
		return 1.0
	}

	mean := w.sum / float64(w.size)
	variance := (w.sumSquares / float64(w.size)) - (mean * mean)
	if variance < 0 || mean == 0 {
		return 1.0
	}

	return math.Sqrt(variance) / mean
}

type covarianceWindow struct {
	xSamples []float64
	ySamples []float64
	size     int
	index    int
	sumX     float64
	sumY     float64
	sumXY    float64
	sumX2    float64
	sumY2    float64
}

func newCovarianceWindow(capacity uint) *covarianceWindow {
	return &covarianceWindow{
		xSamples: make([]float64, capacity),
		ySamples: make([]float64, capacity),
	}
}

// add adds the values to the window and returns the current correlation coefficient.
// Returns a value between .5 and 1 when a correlation between increasing x and y values is present.
// Returns a value between -1 and -.5 when a correlation between increasing x and decreasing y values is present.
// Returns 0 if < 2 samples, low variation (< .01) or weak correlation (< .5).
func (w *covarianceWindow) add(x, y float64) float64 {
	if w.size < len(w.xSamples) {
		w.size++
	} else {
		// Remove old values
		oldX := w.xSamples[w.index]
		oldY := w.ySamples[w.index]
		w.sumX -= oldX
		w.sumY -= oldY
		w.sumXY -= oldX * oldY
		w.sumX2 -= oldX * oldX
		w.sumY2 -= oldY * oldY
	}

	// Add new values
	w.xSamples[w.index] = x
	w.ySamples[w.index] = y
	w.sumX += x
	w.sumY += y
	w.sumXY += x * y
	w.sumX2 += x * x
	w.sumY2 += y * y

	w.index = (w.index + 1) % len(w.xSamples)

	// Require at least 2 values
	if w.size < 2 {
		return 0
	}

	// Calculate means and variances
	meanX := w.sumX / float64(w.size)
	meanY := w.sumY / float64(w.size)
	varX := (w.sumX2 / float64(w.size)) - (meanX * meanX)
	varY := (w.sumY2 / float64(w.size)) - (meanY * meanY)

	// Calculate coefficient of variation (relative variance), which gives us variance as a percentage of the mean
	cvX := math.Sqrt(varX) / math.Abs(meanX)
	cvY := math.Sqrt(varY) / math.Abs(meanY)

	// Ignore measurements that vary by less than 1%
	minCV := 0.01
	if cvX < minCV || cvY < minCV {
		return 0
	}

	// Calculate correlation coefficient
	covariance := (w.sumXY / float64(w.size)) - (meanX * meanY)
	correlation := covariance / math.Sqrt(varX*varY)

	// Ignore weak correlations
	if math.Abs(correlation) < 0.5 {
		return 0
	}

	return correlation
}
