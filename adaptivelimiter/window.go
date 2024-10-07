package adaptivelimiter

import (
	"fmt"
	"math"
	"time"
)

type SampleWindow interface {
	AddSample(rtt int64, inflight int, didDrop bool) SampleWindow
	MinRTTNanos() int64
	AverageRTTNanos() int64
	MaxInFlight() int
	SampleCount() int
	DidDrop() bool
}

type averageSampleWindow struct {
	minRtt      int64
	sum         int64
	maxInFlight int
	sampleCount int
	didDrop     bool
}

func newAverageSampleWindow() *averageSampleWindow {
	return &averageSampleWindow{
		minRtt:      math.MaxInt64,
		sum:         0,
		maxInFlight: 0,
		sampleCount: 0,
		didDrop:     false,
	}
}

// AddSample adds a new sample to the window and returns a new immutable instance
func (w *averageSampleWindow) AddSample(rtt int64, inflight int, didDrop bool) *averageSampleWindow {
	return &averageSampleWindow{
		min(w.minRtt, rtt),
		w.sum + rtt,
		max(inflight, w.maxInFlight),
		w.sampleCount + 1,
		w.didDrop || didDrop,
	}
}

func (w *averageSampleWindow) MinRTTNanos() int64 {
	return w.minRtt
}

func (w *averageSampleWindow) AverageRTTNanos() int64 {
	if w.sampleCount == 0 {
		return 0
	}
	return w.sum / int64(w.sampleCount)
}

func (w *averageSampleWindow) MaxInFlight() int {
	return w.maxInFlight
}

func (w *averageSampleWindow) SampleCount() int {
	return w.sampleCount
}

// DidDrop returns true if any sample had a drop
func (w *averageSampleWindow) DidDrop() bool {
	return w.didDrop
}

// String returns a string representation of the sample window
func (w *averageSampleWindow) String() string {
	return fmt.Sprintf("averageSampleWindow [minRtt=%.3f, avgRtt=%.3f, maxInFlight=%d, sampleCount=%d, didDrop=%v]",
		float64(time.Duration(w.minRtt).Microseconds())/1000.0,
		float64(time.Duration(w.AverageRTTNanos()).Microseconds())/1000.0,
		w.maxInFlight,
		w.sampleCount,
		w.didDrop)
}
