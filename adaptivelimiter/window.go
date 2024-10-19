package adaptivelimiter

import (
	"fmt"
	"math"
	"time"
)

type SampleWindow interface {
	AddSample(rtt time.Duration, inflight uint, didDrop bool) SampleWindow
	MinRTT() time.Duration
	AverageRTT() time.Duration
	MaxInFlight() uint
	SampleCount() uint
	DidDrop() bool
}

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
func (w *averageSampleWindow) AddSample(rtt time.Duration, inflight uint, didDrop bool) SampleWindow {
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

func (w *averageSampleWindow) MinRTT() time.Duration {
	return w.minRTT
}

func (w *averageSampleWindow) AverageRTT() time.Duration {
	if w.sampleCount == 0 {
		return 0
	}
	return w.sum / time.Duration(w.sampleCount)
}

func (w *averageSampleWindow) MaxInFlight() uint {
	return w.maxInFlight
}

func (w *averageSampleWindow) SampleCount() uint {
	return w.sampleCount
}

// DidDrop returns true if any sample had a drop
func (w *averageSampleWindow) DidDrop() bool {
	return w.didDrop
}

// String returns a string representation of the sample window
func (w *averageSampleWindow) String() string {
	return fmt.Sprintf("averageSampleWindow [minRTT=%.3f, avgRtt=%.3f, maxInFlight=%d, sampleCount=%d, didDrop=%v]",
		float64(w.minRTT.Microseconds())/1000.0,
		float64(w.AverageRTT().Microseconds())/1000.0,
		w.maxInFlight,
		w.sampleCount,
		w.didDrop)
}
