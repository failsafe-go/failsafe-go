package adaptivelimiter

type pidStats interface {
	Metrics
	getAndResetStats() (int, int)
	name() string
}

type pidCalibrationWindow struct {
	window      []pidCalibrationPeriod
	size        int
	index       int
	integralSum float64 // Sum of error values over the window
}

func newPidCalibrationWindow(capacity int) *pidCalibrationWindow {
	return &pidCalibrationWindow{window: make([]pidCalibrationPeriod, capacity)}
}

type pidCalibrationPeriod struct {
	inCount  int     // Items that entered the limiter during the calibration period
	outCount int     // Items that exited the limiter during the calibration period
	error    float64 // The computed error for the calibration period
}

func (c *pidCalibrationWindow) add(in, out int, error float64) float64 {
	if c.size < len(c.window) {
		c.size++
	} else {
		c.integralSum -= c.window[c.index].error
	}

	c.integralSum += error
	c.window[c.index] = pidCalibrationPeriod{
		inCount:  in,
		outCount: out,
		error:    error,
	}
	c.index = (c.index + 1) % len(c.window)

	return c.integralSum
}
