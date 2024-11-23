package adaptivelimiter

import (
	"math"
	"time"
)

type rttWindow struct {
	minRTT time.Duration
	rttSum time.Duration
	count  uint
}

func newRTTWindow() *rttWindow {
	return &rttWindow{
		minRTT: math.MaxInt64,
	}
}

// add adds a new sample to the shortRTT and returns a new immutable instance.
func (w *rttWindow) add(rtt time.Duration) *rttWindow {
	return &rttWindow{
		minRTT: min(w.minRTT, rtt),
		rttSum: w.rttSum + rtt,
		count:  w.count + 1,
	}
}

// average returns the average RTT of all samples that have been added to the shortRTT.
func (w *rttWindow) average() time.Duration {
	if w.count == 0 {
		return 0
	}
	return w.rttSum / time.Duration(w.count)
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

func newCovarianceWindow(windowSize uint) *covarianceWindow {
	return &covarianceWindow{
		xSamples: make([]float64, windowSize),
		ySamples: make([]float64, windowSize),
	}
}

// adds the x and y values to the covariance window and returns the current correlation coefficient.
// Returns a value between .5 and 1 when a correlation between the increasing x and y values is present.
// Returns a value between -1 and -.5 when a correlation between increasing x and decreasing y values is present.
// Returns 0 if < 2 samples, low variation (< .01) or weak correlation (< .5).
func (c *covarianceWindow) add(x, y float64) float64 {
	if c.size < len(c.xSamples) {
		c.size++
	} else {
		oldX := c.xSamples[c.index]
		oldY := c.ySamples[c.index]
		c.sumX -= oldX
		c.sumY -= oldY
		c.sumXY -= oldX * oldY
		c.sumX2 -= oldX * oldX
		c.sumY2 -= oldY * oldY
	}

	c.xSamples[c.index] = x
	c.ySamples[c.index] = y
	c.sumX += x
	c.sumY += y
	c.sumXY += x * y
	c.sumX2 += x * x
	c.sumY2 += y * y

	c.index = (c.index + 1) % len(c.xSamples)

	if c.size < 2 {
		return 0
	}

	// Calculate means
	meanX := c.sumX / float64(c.size)
	meanY := c.sumY / float64(c.size)

	// Calculate variances
	varX := (c.sumX2 / float64(c.size)) - (meanX * meanX)
	varY := (c.sumY2 / float64(c.size)) - (meanY * meanY)

	// Calculate coefficient of variation (relative variance)
	// This gives us variance as a percentage of the mean
	cvX := math.Sqrt(varX) / math.Abs(meanX)
	cvY := math.Sqrt(varY) / math.Abs(meanY)

	// Ignore measurements that vary by less than 1%
	minCV := 0.01
	if cvX < minCV || cvY < minCV {
		return 0
	}

	// Calculate correlation coefficient
	covariance := (c.sumXY / float64(c.size)) - (meanX * meanY)
	correlation := covariance / math.Sqrt(varX*varY)

	// Ignore weak correlations
	if math.Abs(correlation) < 0.5 {
		return 0
	}

	return correlation
}

type variationWindow struct {
	samples      []float64
	index        int
	sum          float64
	sumSquares   float64
	nonZeroCount float64 // Count of non-zero samples in shortRTT
}

func newVariationWindow(windowSize int) *variationWindow {
	return &variationWindow{
		samples: make([]float64, windowSize),
	}
}

// add returns the coefficient of variation (relative standard deviation)
// for a shortRTT of samples. Lower values indicate more stability.
func (s *variationWindow) add(value float64) float64 {
	// Remove old sample contribution if non-zero
	if old := s.samples[s.index]; old != 0 {
		s.sum -= old
		s.sumSquares -= old * old
		s.nonZeroCount--
	}

	// add new sample contribution if non-zero
	s.samples[s.index] = value
	if value != 0 {
		s.sum += value
		s.sumSquares += value * value
		s.nonZeroCount++
	}

	s.index = (s.index + 1) % len(s.samples)

	// Need at least 2 non-zero samples for meaningful calculation
	if s.nonZeroCount < 2 {
		return 1.0
	}

	mean := s.sum / s.nonZeroCount
	variance := (s.sumSquares / s.nonZeroCount) - (mean * mean)
	if variance < 0 || mean == 0 {
		return 1.0
	}

	return math.Sqrt(variance) / mean
}
