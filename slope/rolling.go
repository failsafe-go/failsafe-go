package slope

// RollingSlope maintains a fixed-size window of values and calculates the slope
// of the linear regression line through those points in O(1) time complexity
type RollingSlope struct {
	samples    []float64 // Fixed size array
	index      int       // Reading position
	count      int       // Number of values in window
	windowSize int
	sumY       float64
	sumXY      float64
}

// NewRollingSlope creates a new RollingSlope instance with the specified window size
func NewRollingSlope(windowSize int) *RollingSlope {
	return &RollingSlope{
		samples:    make([]float64, windowSize),
		index:      0,
		count:      0,
		windowSize: windowSize,
		sumY:       0,
		sumXY:      0,
	}
}

// AddValue adds a new value to the window and returns the current slope
// Returns nil if the window is not yet full
func (rs *RollingSlope) AddValue(y float64) float64 {
	if rs.count == rs.windowSize {
		// Remove oldest value
		rs.sumY -= rs.samples[rs.index]

		// Shift all values left (via telescoping series)
		rs.sumXY = rs.sumXY - rs.sumY

		// Add new value at the end
		rs.sumXY += float64(rs.count-1) * y
	} else {
		// Still building up the window
		rs.sumXY += y * float64(rs.count)
		rs.count++
	}

	rs.samples[rs.index] = y
	rs.sumY += y

	// Move index forward
	rs.index = (rs.index + 1) % rs.windowSize

	if rs.count < 2 {
		return 0
	}

	// Calculate slope using least squares
	n := float64(rs.count)
	sumX := n * (n - 1) / 2
	sumXSquared := n * (n - 1) * (2*n - 1) / 6
	slope := (n*rs.sumXY - sumX*rs.sumY) / (n*sumXSquared - sumX*sumX)
	return slope
}
