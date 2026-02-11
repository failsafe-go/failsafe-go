package util

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/influxdata/tdigest"
)

// accuracyStats tracks accuracy metrics for a quantile estimator
type accuracyStats struct {
	name              string
	errors            []float64 // absolute errors at each sample point
	percentErrors     []float64 // percentage errors at each sample point
	meanError         float64
	meanPercentError  float64
	maxError          float64
	maxPercentError   float64
	percentile95      float64 // 95th percentile of errors
	percentile99      float64 // 99th percentile of errors
	stableErrors      []float64 // errors after warmup period
	stableMeanError   float64
	stableMeanPercent float64
}

func (s *accuracyStats) calculate() {
	if len(s.errors) == 0 {
		return
	}

	// Calculate mean absolute error
	sum := 0.0
	for _, e := range s.errors {
		sum += e
	}
	s.meanError = sum / float64(len(s.errors))

	// Calculate mean percentage error
	percentSum := 0.0
	for _, e := range s.percentErrors {
		percentSum += e
	}
	s.meanPercentError = percentSum / float64(len(s.percentErrors))

	// Find max absolute error
	s.maxError = s.errors[0]
	for _, e := range s.errors {
		if e > s.maxError {
			s.maxError = e
		}
	}

	// Find max percentage error
	if len(s.percentErrors) > 0 {
		s.maxPercentError = s.percentErrors[0]
		for _, e := range s.percentErrors {
			if e > s.maxPercentError {
				s.maxPercentError = e
			}
		}
	}

	// Calculate percentiles
	sorted := make([]float64, len(s.errors))
	copy(sorted, s.errors)
	sort.Float64s(sorted)
	s.percentile95 = sorted[int(float64(len(sorted))*0.95)]
	s.percentile99 = sorted[int(float64(len(sorted))*0.99)]

	// Calculate stable period stats (after warmup)
	if len(s.stableErrors) > 0 {
		stableSum := 0.0
		for _, e := range s.stableErrors {
			stableSum += e
		}
		s.stableMeanError = stableSum / float64(len(s.stableErrors))
	}
}

func (s *accuracyStats) String() string {
	if s.meanPercentError == 0 && s.meanError == 0 {
		return fmt.Sprintf("%s:\n"+
			"  Mean Error:       0.00%% (EXACT) ✨\n"+
			"  Max Error:        0.00%% (EXACT) ✨\n"+
			"  Samples:          %d",
			s.name, len(s.errors))
	}
	return fmt.Sprintf("%s:\n"+
		"  Mean Error:       %.2f%% (absolute: %.2f)\n"+
		"  Max Error:        %.2f%% (absolute: %.2f)\n"+
		"  Samples:          %d",
		s.name, s.meanPercentError, s.meanError, s.maxPercentError, s.maxError, len(s.errors))
}

// calculateTrueQuantile calculates the exact quantile from a sorted window
func calculateTrueQuantile(window []float64, quantile float64) float64 {
	if len(window) == 0 {
		return 0
	}
	sorted := make([]float64, len(window))
	copy(sorted, window)
	sort.Float64s(sorted)
	pos := int(float64(len(sorted)-1) * quantile)
	return sorted[pos]
}

// timestampedSample represents a sample with its timestamp for ground truth comparison
type timestampedSample struct {
	value     float64
	timestamp time.Time
}

// TestAccuracy_OverTime_UniformDistribution tests accuracy with uniformly distributed values
func TestAccuracy_OverTime_UniformDistribution(t *testing.T) {
	const (
		numSamples     = 10000
		windowDuration = 1000 * time.Second
		windowSize     = 1000 // For ground truth window
		quantile       = 0.9
		warmupSize     = windowSize * 2 // After this, we track stable errors
	)

	t.Logf("Testing accuracy over %d samples with window duration %v, tracking p%.0f", numSamples, windowDuration, quantile*100)
	t.Logf("Distribution: Uniform [0, 1000)")

	// Initialize data structures
	qw := NewQuantileWindow(quantile, windowDuration)
	mq := NewMovingQuantile(quantile, 0.01, 100)
	td := tdigest.NewWithCompression(100)

	// Track ground truth window (time-based like QuantileWindow)
	allSamples := make([]timestampedSample, 0, numSamples)
	baseTime := time.Now()

	// Stats collectors
	qwStats := &accuracyStats{name: "QuantileWindow (exact)"}
	mqStats := &accuracyStats{name: "MovingQuantile (EMA)"}
	tdStats := &accuracyStats{name: "TDigest (approximate)"}

	// Generate uniform random samples (1 sample per second)
	rand.Seed(42)
	for i := 0; i < numSamples; i++ {
		value := rand.Float64() * 1000
		timestamp := baseTime.Add(time.Duration(i) * time.Second)

		// Add to all implementations
		qw.AddWithTime(value, timestamp)
		mq.Add(value)
		td.Add(value, 1)

		// Track all samples with timestamps
		allSamples = append(allSamples, timestampedSample{value, timestamp})

		// Build time-based ground truth window (samples within windowDuration)
		cutoff := timestamp.Add(-windowDuration)
		var trueWindow []float64
		for _, s := range allSamples {
			if !s.timestamp.Before(cutoff) {
				trueWindow = append(trueWindow, s.value)
			}
		}

		// Calculate true quantile and compare (after window fills)
		if len(trueWindow) >= windowSize {
			trueValue := calculateTrueQuantile(trueWindow, quantile)

			qwError := math.Abs(qw.Value() - trueValue)
			mqError := math.Abs(mq.Value() - trueValue)
			tdError := math.Abs(td.Quantile(quantile) - trueValue)

			// Calculate percentage errors (avoid division by zero)
			qwPercentError := 0.0
			mqPercentError := 0.0
			tdPercentError := 0.0
			if trueValue > 0 {
				qwPercentError = (qwError / trueValue) * 100
				mqPercentError = (mqError / trueValue) * 100
				tdPercentError = (tdError / trueValue) * 100
			}

			qwStats.errors = append(qwStats.errors, qwError)
			qwStats.percentErrors = append(qwStats.percentErrors, qwPercentError)
			mqStats.errors = append(mqStats.errors, mqError)
			mqStats.percentErrors = append(mqStats.percentErrors, mqPercentError)
			tdStats.errors = append(tdStats.errors, tdError)
			tdStats.percentErrors = append(tdStats.percentErrors, tdPercentError)

			// Track stable period (after warmup)
			if i >= warmupSize {
				qwStats.stableErrors = append(qwStats.stableErrors, qwError)
				mqStats.stableErrors = append(mqStats.stableErrors, mqError)
				tdStats.stableErrors = append(tdStats.stableErrors, tdError)
			}
		}
	}

	// Calculate and print stats
	qwStats.calculate()
	mqStats.calculate()
	tdStats.calculate()

	t.Log("\n" + qwStats.String())
	t.Log("\n" + mqStats.String())
	t.Log("\n" + tdStats.String())

	// Verify QuantileWindow has zero error (it's exact)
	if qwStats.maxError > 0.001 {
		t.Errorf("QuantileWindow should be exact, but has max error %.4f", qwStats.maxError)
	}
}

// TestAccuracy_OverTime_NormalDistribution tests accuracy with normally distributed values
func TestAccuracy_OverTime_NormalDistribution(t *testing.T) {
	const (
		numSamples     = 10000
		windowDuration = 1000 * time.Second
		windowSize     = 1000
		quantile       = 0.9
		warmupSize     = windowSize * 2
	)

	t.Logf("Testing accuracy over %d samples with window duration %v, tracking p%.0f", numSamples, windowDuration, quantile*100)
	t.Logf("Distribution: Normal (mean=100, stddev=20)")

	qw := NewQuantileWindow(quantile, windowDuration)
	mq := NewMovingQuantile(quantile, 0.01, 100)
	td := tdigest.NewWithCompression(100)
	allSamples := make([]timestampedSample, 0, numSamples)
	baseTime := time.Now()

	qwStats := &accuracyStats{name: "QuantileWindow (exact)"}
	mqStats := &accuracyStats{name: "MovingQuantile (EMA)"}
	tdStats := &accuracyStats{name: "TDigest (approximate)"}

	rand.Seed(42)
	for i := 0; i < numSamples; i++ {
		// Box-Muller transform for normal distribution
		u1 := rand.Float64()
		u2 := rand.Float64()
		z0 := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
		value := 100 + 20*z0 // mean=100, stddev=20
		timestamp := baseTime.Add(time.Duration(i) * time.Second)

		qw.AddWithTime(value, timestamp)
		mq.Add(value)
		td.Add(value, 1)

		allSamples = append(allSamples, timestampedSample{value, timestamp})

		// Build time-based ground truth window
		cutoff := timestamp.Add(-windowDuration)
		var trueWindow []float64
		for _, s := range allSamples {
			if !s.timestamp.Before(cutoff) {
				trueWindow = append(trueWindow, s.value)
			}
		}

		if len(trueWindow) >= windowSize {
			trueValue := calculateTrueQuantile(trueWindow, quantile)

			qwError := math.Abs(qw.Value() - trueValue)
			mqError := math.Abs(mq.Value() - trueValue)
			tdError := math.Abs(td.Quantile(quantile) - trueValue)

			// Calculate percentage errors
			qwPercentError, mqPercentError, tdPercentError := 0.0, 0.0, 0.0
			if trueValue > 0 {
				qwPercentError = (qwError / trueValue) * 100
				mqPercentError = (mqError / trueValue) * 100
				tdPercentError = (tdError / trueValue) * 100
			}

			qwStats.errors = append(qwStats.errors, qwError)
			qwStats.percentErrors = append(qwStats.percentErrors, qwPercentError)
			mqStats.errors = append(mqStats.errors, mqError)
			mqStats.percentErrors = append(mqStats.percentErrors, mqPercentError)
			tdStats.errors = append(tdStats.errors, tdError)
			tdStats.percentErrors = append(tdStats.percentErrors, tdPercentError)

			if i >= warmupSize {
				qwStats.stableErrors = append(qwStats.stableErrors, qwError)
				mqStats.stableErrors = append(mqStats.stableErrors, mqError)
				tdStats.stableErrors = append(tdStats.stableErrors, tdError)
			}
		}
	}

	qwStats.calculate()
	mqStats.calculate()
	tdStats.calculate()

	t.Log("\n" + qwStats.String())
	t.Log("\n" + mqStats.String())
	t.Log("\n" + tdStats.String())

	if qwStats.maxError > 0.001 {
		t.Errorf("QuantileWindow should be exact, but has max error %.4f", qwStats.maxError)
	}
}

// TestAccuracy_OverTime_BimodalDistribution tests with bimodal (fast/slow) distribution
func TestAccuracy_OverTime_BimodalDistribution(t *testing.T) {
	const (
		numSamples     = 10000
		windowDuration = 1000 * time.Second
		windowSize     = 1000
		quantile       = 0.9
		warmupSize     = windowSize * 2
	)

	t.Logf("Testing accuracy over %d samples with window duration %v, tracking p%.0f", numSamples, windowDuration, quantile*100)
	t.Logf("Distribution: Bimodal (80%% fast ~20ms, 20%% slow ~200ms)")

	qw := NewQuantileWindow(quantile, windowDuration)
	mq := NewMovingQuantile(quantile, 0.01, 100)
	td := tdigest.NewWithCompression(100)
	allSamples := make([]timestampedSample, 0, numSamples)
	baseTime := time.Now()

	qwStats := &accuracyStats{name: "QuantileWindow (exact)"}
	mqStats := &accuracyStats{name: "MovingQuantile (EMA)"}
	tdStats := &accuracyStats{name: "TDigest (approximate)"}

	rand.Seed(42)
	for i := 0; i < numSamples; i++ {
		var value float64
		if rand.Float64() < 0.8 {
			// Fast requests (80%)
			value = 20 + rand.Float64()*10 // 20-30ms
		} else {
			// Slow requests (20%)
			value = 200 + rand.Float64()*50 // 200-250ms
		}
		timestamp := baseTime.Add(time.Duration(i) * time.Second)

		qw.AddWithTime(value, timestamp)
		mq.Add(value)
		td.Add(value, 1)

		allSamples = append(allSamples, timestampedSample{value, timestamp})

		// Build time-based ground truth window
		cutoff := timestamp.Add(-windowDuration)
		var trueWindow []float64
		for _, s := range allSamples {
			if !s.timestamp.Before(cutoff) {
				trueWindow = append(trueWindow, s.value)
			}
		}

		if len(trueWindow) >= windowSize {
			trueValue := calculateTrueQuantile(trueWindow, quantile)

			qwError := math.Abs(qw.Value() - trueValue)
			mqError := math.Abs(mq.Value() - trueValue)
			tdError := math.Abs(td.Quantile(quantile) - trueValue)

			qwStats.errors = append(qwStats.errors, qwError)
			mqStats.errors = append(mqStats.errors, mqError)
			tdStats.errors = append(tdStats.errors, tdError)

			if i >= warmupSize {
				qwStats.stableErrors = append(qwStats.stableErrors, qwError)
				mqStats.stableErrors = append(mqStats.stableErrors, mqError)
				tdStats.stableErrors = append(tdStats.stableErrors, tdError)
			}
		}
	}

	qwStats.calculate()
	mqStats.calculate()
	tdStats.calculate()

	t.Log("\n" + qwStats.String())
	t.Log("\n" + mqStats.String())
	t.Log("\n" + tdStats.String())

	if qwStats.maxError > 0.001 {
		t.Errorf("QuantileWindow should be exact, but has max error %.4f", qwStats.maxError)
	}
}

// TestAccuracy_OverTime_ShiftingDistribution tests with a distribution that shifts over time
func TestAccuracy_OverTime_ShiftingDistribution(t *testing.T) {
	const (
		numSamples     = 10000
		windowDuration = 1000 * time.Second
		windowSize     = 1000
		quantile       = 0.9
		warmupSize     = windowSize * 2
	)

	t.Logf("Testing accuracy over %d samples with window duration %v, tracking p%.0f", numSamples, windowDuration, quantile*100)
	t.Logf("Distribution: Shifting (mean drifts from 50 to 150 over time)")

	qw := NewQuantileWindow(quantile, windowDuration)
	mq := NewMovingQuantile(quantile, 0.01, 100)
	td := tdigest.NewWithCompression(100)
	allSamples := make([]timestampedSample, 0, numSamples)
	baseTime := time.Now()

	qwStats := &accuracyStats{name: "QuantileWindow (exact)"}
	mqStats := &accuracyStats{name: "MovingQuantile (EMA)"}
	tdStats := &accuracyStats{name: "TDigest (approximate)"}

	rand.Seed(42)
	for i := 0; i < numSamples; i++ {
		// Mean shifts linearly from 50 to 150
		mean := 50 + (100 * float64(i) / float64(numSamples))
		value := mean + (rand.Float64()-0.5)*20 // +/- 10 around mean
		timestamp := baseTime.Add(time.Duration(i) * time.Second)

		qw.AddWithTime(value, timestamp)
		mq.Add(value)
		td.Add(value, 1)

		allSamples = append(allSamples, timestampedSample{value, timestamp})

		// Build time-based ground truth window
		cutoff := timestamp.Add(-windowDuration)
		var trueWindow []float64
		for _, s := range allSamples {
			if !s.timestamp.Before(cutoff) {
				trueWindow = append(trueWindow, s.value)
			}
		}

		if len(trueWindow) >= windowSize {
			trueValue := calculateTrueQuantile(trueWindow, quantile)

			qwError := math.Abs(qw.Value() - trueValue)
			mqError := math.Abs(mq.Value() - trueValue)
			tdError := math.Abs(td.Quantile(quantile) - trueValue)

			qwStats.errors = append(qwStats.errors, qwError)
			mqStats.errors = append(mqStats.errors, mqError)
			tdStats.errors = append(tdStats.errors, tdError)

			if i >= warmupSize {
				qwStats.stableErrors = append(qwStats.stableErrors, qwError)
				mqStats.stableErrors = append(mqStats.stableErrors, mqError)
				tdStats.stableErrors = append(tdStats.stableErrors, tdError)
			}
		}
	}

	qwStats.calculate()
	mqStats.calculate()
	tdStats.calculate()

	t.Log("\n" + qwStats.String())
	t.Log("\n" + mqStats.String())
	t.Log("\n" + tdStats.String())

	if qwStats.maxError > 0.001 {
		t.Errorf("QuantileWindow should be exact, but has max error %.4f", qwStats.maxError)
	}

	t.Logf("\nNote: MovingQuantile uses EMA decay, so it may lag during distribution shifts.")
	t.Logf("TDigest has no sliding window, so it accumulates all historical data.")
}

// TestAccuracy_CompareQuantiles tests accuracy across different quantile values
func TestAccuracy_CompareQuantiles(t *testing.T) {
	const (
		numSamples     = 5000
		windowDuration = 1000 * time.Second
		windowSize     = 1000
	)

	quantiles := []float64{0.5, 0.75, 0.9, 0.95, 0.99}
	rand.Seed(42)

	// Pre-generate samples
	samples := make([]float64, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i] = rand.Float64() * 1000
	}

	t.Logf("Testing accuracy across different quantiles (%d samples, window duration %v)\n", numSamples, windowDuration)

	for _, q := range quantiles {
		qw := NewQuantileWindow(q, windowDuration)
		mq := NewMovingQuantile(q, 0.01, 100)
		td := tdigest.NewWithCompression(100)
		allSamples := make([]timestampedSample, 0, numSamples)
		baseTime := time.Now()

		var qwTotalError, mqTotalError, tdTotalError float64
		var count int

		for i, value := range samples {
			timestamp := baseTime.Add(time.Duration(i) * time.Second)
			qw.AddWithTime(value, timestamp)
			mq.Add(value)
			td.Add(value, 1)

			allSamples = append(allSamples, timestampedSample{value, timestamp})

			// Build time-based ground truth window
			cutoff := timestamp.Add(-windowDuration)
			var trueWindow []float64
			for _, s := range allSamples {
				if !s.timestamp.Before(cutoff) {
					trueWindow = append(trueWindow, s.value)
				}
			}

			if len(trueWindow) >= windowSize {
				trueValue := calculateTrueQuantile(trueWindow, q)
				qwTotalError += math.Abs(qw.Value() - trueValue)
				mqTotalError += math.Abs(mq.Value() - trueValue)
				tdTotalError += math.Abs(td.Quantile(q) - trueValue)
				count++
			}
		}

		t.Logf("p%.0f: QW=%.4f  MQ=%.4f  TD=%.4f (mean absolute error)",
			q*100,
			qwTotalError/float64(count),
			mqTotalError/float64(count),
			tdTotalError/float64(count))
	}
}
