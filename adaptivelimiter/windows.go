package adaptivelimiter

import (
	"fmt"
	"math"
	"time"
)

type averageSampleWindow struct {
	minRTT      time.Duration
	sum         time.Duration
	maxInFlight uint
	sampleCount uint
	didDrop     bool
}

func newAverageSampleWindow() *averageSampleWindow {
	return &averageSampleWindow{
		minRTT:      math.MaxInt64,
		sum:         0,
		maxInFlight: 0,
		sampleCount: 0,
		didDrop:     false,
	}
}

// AddSample adds a new sample to the window and returns a new immutable instance
func (w *averageSampleWindow) AddSample(rtt time.Duration, inflight uint, didDrop bool) *averageSampleWindow {
	minRTT := w.minRTT
	sum := w.sum
	if !didDrop {
		minRTT = min(w.minRTT, rtt)
		sum = w.sum + rtt
	}
	return &averageSampleWindow{
		minRTT,
		sum,
		max(inflight, w.maxInFlight),
		w.sampleCount + 1,
		w.didDrop || didDrop,
	}
}

func (w *averageSampleWindow) AverageRTT() time.Duration {
	if w.sampleCount == 0 {
		return 0
	}
	return w.sum / time.Duration(w.sampleCount)
}

func (w *averageSampleWindow) String() string {
	return fmt.Sprintf("averageSampleWindow [minRTT=%.3f, avgRtt=%.3f, maxInFlight=%d, sampleCount=%d, didDrop=%v]",
		float64(w.minRTT.Milliseconds()),
		float64(w.AverageRTT().Milliseconds()),
		w.maxInFlight,
		w.sampleCount,
		w.didDrop)
}

type covarianceWindow struct {
	windowSize uint

	// Mutable state
	sampleCount uint
	sumX        float64
	sumY        float64
	sumXY       float64
	sumX2       float64
	sumY2       float64
	xSamples    []float64
	ySamples    []float64
	index       uint
}

func newCovarianceWindow(windowSize uint) *covarianceWindow {
	return &covarianceWindow{
		windowSize: windowSize,
		xSamples:   make([]float64, windowSize),
		ySamples:   make([]float64, windowSize),
	}
}

// Add adds the x and y values to the covariance window and returns the current covariance.
func (c *covarianceWindow) Add(x, y float64) float64 {
	if c.sampleCount < c.windowSize {
		c.sampleCount++
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

	c.index = (c.index + 1) % c.windowSize

	if c.sampleCount < 2 {
		return 0
	}
	return (c.sumXY - (c.sumX * c.sumY / float64(c.sampleCount))) / float64(c.sampleCount)
}
