package adaptivelimiter

import (
	"math"
	"time"
)

type sampleWindow struct {
	minRTT  time.Duration
	rttSum  time.Duration
	count   uint
	didDrop bool
}

func newSampleWindow() *sampleWindow {
	return &sampleWindow{
		minRTT: math.MaxInt64,
	}
}

// AddSample adds a new sample to the window and returns a new immutable instance.
func (w *sampleWindow) AddSample(rtt time.Duration, didDrop bool) *sampleWindow {
	minRTT := w.minRTT
	rttSum := w.rttSum
	if !didDrop {
		minRTT = min(w.minRTT, rtt)
		rttSum = w.rttSum + rtt
	}
	return &sampleWindow{
		minRTT:  minRTT,
		rttSum:  rttSum,
		count:   w.count + 1,
		didDrop: w.didDrop || didDrop,
	}
}

// AverageRTT returns the average RTT of all samples that have been added to the window.
func (w *sampleWindow) AverageRTT() time.Duration {
	if w.count == 0 {
		return 0
	}
	return w.rttSum / time.Duration(w.count)
}

type covarianceWindow struct {
	size     int
	index    int
	sumX     float64
	sumY     float64
	sumXY    float64
	sumX2    float64
	sumY2    float64
	xSamples []float64
	ySamples []float64
}

func newCovarianceWindow(windowSize uint) *covarianceWindow {
	return &covarianceWindow{
		xSamples: make([]float64, windowSize),
		ySamples: make([]float64, windowSize),
	}
}

// Add adds the x and y values to the covariance window and returns the current covariance.
func (c *covarianceWindow) Add(x, y float64) float64 {
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
	return (c.sumXY - (c.sumX * c.sumY / float64(c.size))) / float64(c.size)
}
