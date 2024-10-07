package adaptivethrottler

// TODO my own notes
// this appears to just be a limiter that keeps track of recent successes/failures using the EMA
// then it either increases or decreases the concurrencylimit based on some thresholds
// this is similar, but less effective to a gradient IMo
// AdaptiveConcurrencyLimiter adjusts concurrency based on failure rates.

// TODO
// a limiter vs a throttler
// adaptive concurrency limiter
// a limiter limits concurrency up and down based on recent failure rates or latency
// adaptive execution throttler
// a throttler rejects individual requests based on recent failure rates
//type AdaptiveConcurrencyLimiter struct {
//	mu sync.Mutex
//
//	// Concurrency limit variables
//	concurrencyLimit int
//	maxLimit         int
//	minLimit         int
//
//	// Current number of in-flight requests
//	inFlight int
//
//	// Failure rate tracking
//	failureRateEMA float64
//	alpha          float64 // Smoothing factor for EMA
//
//	// Thresholds for adjusting concurrency limit
//	highFailureThreshold float64
//	lowFailureThreshold  float64
//
//	// Adjustment amounts
//	increaseStep int
//	decreaseStep int
//}
//
//// NewAdaptiveConcurrencyLimiter initializes a new limiter.
//func NewAdaptiveConcurrencyLimiter(initialLimit, minLimit, maxLimit int) *AdaptiveConcurrencyLimiter {
//	return &AdaptiveConcurrencyLimiter{
//		concurrencyLimit:     initialLimit,
//		maxLimit:             maxLimit,
//		minLimit:             minLimit,
//		failureRateEMA:       0.0,
//		alpha:                0.1,  // Adjust as needed for smoothing
//		highFailureThreshold: 0.2,  // Adjust based on acceptable failure rate
//		lowFailureThreshold:  0.05, // Adjust based on acceptable failure rate
//		increaseStep:         1,
//		decreaseStep:         1,
//	}
//}
//
//// TryAcquire attempts to acquire a slot to proceed with a request.
//func (l *AdaptiveConcurrencyLimiter) TryAcquirePermit() bool {
//	l.mu.Lock()
//	defer l.mu.Unlock()
//
//	if l.inFlight < l.concurrencyLimit {
//		l.inFlight++
//		return true
//	}
//	return false
//}
//
////// Release should be called when a request has completed.
////func (l *AdaptiveConcurrencyLimiter) ReleasePermit() {
////	l.mu.Lock()
////	defer l.mu.Unlock()
////
////	if l.inFlight > 0 {
////		l.inFlight--
////	}
////}
//
//// RecordResult records the outcome of a request and updates the failure rate.
//func (l *AdaptiveConcurrencyLimiter) RecordResult(success bool) {
//	l.mu.Lock()
//	defer l.mu.Unlock()
//
//	if l.inFlight > 0 {
//		l.inFlight--
//	}
//
//	// Update the exponential moving average of the failure rate.
//	var failure float64
//	if !success {
//		failure = 1.0
//	} else {
//		failure = 0.0
//	}
//	l.failureRateEMA = l.alpha*failure + (1-l.alpha)*l.failureRateEMA
//
//	// Adjust concurrency limit based on failure rate.
//	l.adjustConcurrencyLimit()
//}
//
//// adjustConcurrencyLimit adjusts the concurrency limit based on the failure rate.
//func (l *AdaptiveConcurrencyLimiter) adjustConcurrencyLimit() {
//	if l.failureRateEMA > l.highFailureThreshold {
//		// Decrease concurrency limit.
//		l.concurrencyLimit = max(l.minLimit, l.concurrencyLimit-l.decreaseStep)
//	} else if l.failureRateEMA < l.lowFailureThreshold {
//		// Increase concurrency limit.
//		l.concurrencyLimit = min(l.maxLimit, l.concurrencyLimit+l.increaseStep)
//	}
//}
//
//// GetCurrentLimit returns the current concurrency limit.
//func (l *AdaptiveConcurrencyLimiter) GetCurrentLimit() int {
//	l.mu.Lock()
//	defer l.mu.Unlock()
//	return l.concurrencyLimit
//}
