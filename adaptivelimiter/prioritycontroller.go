package adaptivelimiter

//
// import (
// 	"context"
// 	"fmt"
// 	"sync"
// 	"sync/atomic"
// 	"time"
//
// 	"github.com/failsafe-go/failsafe-go"
// 	"github.com/failsafe-go/failsafe-go/internal/util"
// )
//
// const (
// 	PriorityLowest  = 4
// 	PriorityLow     = 3
// 	PriorityMedium  = 2
// 	PriorityHigh    = 1
// 	PriorityHighest = 0
// )
//
// type PriorityLimiter[R any] interface {
// 	failsafe.Policy[R]
//
// 	// AcquirePermit attempts to acquire a permit, potentially blocking up to maxExecutionTime.
// 	// The request priority must be less than the current priority threshold for admission.
// 	AcquirePermit(ctx context.Context, priority int) (Permit, error)
//
// 	// PriorityThreshold returns the current minimum priority required for admission
// 	PriorityThreshold() int
// }
//
// type priorityBlockingLimiter[R any] struct {
// 	*adaptiveLimiter[R]
// 	maxExecutionTime  time.Duration
// 	priorityThreshold int
//
// 	mu           sync.Mutex
// 	blockedCount int
//
// 	// Track request flow
// 	inCount  atomic.Uint32
// 	outCount atomic.Uint32
//
// 	// PID controller state
// 	kp           float64
// 	ki           float64
// 	calibrations *calibrationWindow
// }
//
// func (l *priorityBlockingLimiter[R]) AcquirePermit(ctx context.Context, priority int) (Permit, error) {
// 	// Reject if priority is higher (lower importance) than threshold
// 	l.mu.Lock()
// 	if priority > l.priorityThreshold {
// 		l.mu.Unlock()
// 		return nil, ErrExceeded
// 	}
//
// 	l.inCount.Add(1)
//
// 	// Try without waiting first
// 	if permit, ok := l.adaptiveLimiter.TryAcquirePermit(); ok {
// 		l.outCount.Add(1)
// 		return permit, nil
// 	}
//
// 	// Always reject if over maxExecutionTime
// 	estimatedLatency := l.estimateLatency()
// 	if estimatedLatency > l.maxExecutionTime {
// 		return nil, ErrExceeded
// 	}
//
// 	// Block waiting for permit
// 	l.blockedCount++
// 	l.mu.Unlock()
//
// 	permit, err := l.adaptiveLimiter.AcquirePermit(ctx)
// 	if err != nil {
// 		l.mu.Lock()
// 		l.blockedCount--
// 		l.mu.Unlock()
// 		return nil, err
// 	}
//
// 	l.outCount.Add(1)
// 	l.mu.Lock()
// 	l.blockedCount--
// 	l.mu.Unlock()
//
// 	return permit, nil
// }
//
// func (l *priorityBlockingLimiter[R]) PriorityThreshold() int {
// 	l.mu.Lock()
// 	defer l.mu.Unlock()
// 	return l.priorityThreshold
// }
//
// func (l *priorityBlockingLimiter[R]) BlockedCount() int {
// 	l.mu.Lock()
// 	defer l.mu.Unlock()
// 	return l.blockedCount
// }
//
// // ScheduleCalibrations runs calibration on an interval
// func (l *priorityBlockingLimiter[R]) ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc {
// 	ticker := time.NewTicker(interval)
// 	done := make(chan struct{})
//
// 	go func() {
// 		defer ticker.Stop()
// 		for {
// 			select {
// 			case <-ctx.Done():
// 				return
// 			case <-done:
// 				return
// 			case <-ticker.C:
// 				l.Calibrate()
// 			}
// 		}
// 	}()
//
// 	return func() {
// 		close(done)
// 	}
// }
//
// // Calibrate adjusts the priority threshold based on request flow metrics
// func (l *priorityBlockingLimiter[R]) Calibrate() {
// 	inCount := int(l.inCount.Swap(0))
// 	outCount := int(l.outCount.Swap(0))
// 	freeInflight := l.Limit() - l.Inflight()
//
// 	l.mu.Lock()
// 	defer l.mu.Unlock()
//
// 	// Get both P value and integral sum from calibration window
// 	pValue, integralSum := l.calibrations.add(inCount, outCount, freeInflight, l.Limit())
//
// 	// Calculate PID adjustment
// 	adjustment := l.kp*pValue + l.ki*integralSum
//
// 	// Convert to threshold change
// 	currentThreshold := l.priorityThreshold
// 	newThreshold := currentThreshold
//
// 	// Only change threshold on significant adjustments
// 	if adjustment > 0.5 && currentThreshold > PriorityHighest {
// 		newThreshold--
// 	} else if adjustment < -0.5 && currentThreshold < PriorityLowest {
// 		newThreshold++
// 	}
//
// 	if l.logger != nil && newThreshold != currentThreshold {
// 		l.logger.Debug("updated priority threshold",
// 			"oldThreshold", currentThreshold,
// 			"newThreshold", newThreshold,
// 			"pValue", fmt.Sprintf("%.2f", pValue),
// 			"integral", fmt.Sprintf("%.2f", integralSum),
// 			"adjustment", fmt.Sprintf("%.2f", adjustment),
// 			"inCount", inCount,
// 			"outCount", outCount)
// 	}
//
// 	l.priorityThreshold = newThreshold
// }
//
// // estimateLatency estimates wait time for a new request based on current conditions
// func (l *priorityBlockingLimiter[R]) estimateLatency() time.Duration {
// 	avgProcessing := time.Duration(l.longRTT.Value())
// 	if avgProcessing == 0 {
// 		avgProcessing = l.maxExecutionTime / warmupSamples
// 	}
//
// 	totalRequests := l.blockedCount + 1
// 	concurrency := l.Limit()
// 	fullBatches := totalRequests / concurrency
// 	if totalRequests%concurrency > 0 {
// 		fullBatches++
// 	}
//
// 	return time.Duration(float64(fullBatches) * float64(avgProcessing))
// }
//
// type calibrationWindow struct {
// 	window       []calibrationPeriod
// 	size         int
// 	head         int
// 	integralEWMA util.MovingAverage
// }
//
// type calibrationPeriod struct {
// 	inCount  int
// 	outCount int
// 	pValue   float64
// }
//
// func (c *calibrationWindow) add(in, out, freeInflight int, limit int) (pValue float64, integralSum float64) {
// 	if c.size < len(c.window) {
// 		c.size++
// 	}
//
// 	// Calculate P value from request flow metrics
// 	pValue = computePValue(in, out, freeInflight, limit)
//
// 	// Use EWMA for integral term instead of unbounded sum
// 	// This provides a bounded history window and weights recent values more heavily
// 	integralSum = c.integralEWMA.Add(pValue)
//
// 	c.window[c.head] = calibrationPeriod{
// 		inCount:  in,
// 		outCount: out,
// 		pValue:   pValue,
// 	}
// 	c.head = (c.head + 1) % len(c.window)
// 	return pValue, integralSum
// }
//
// // Computes P value from request flow metrics.
// // A positive P value indicates overload, negative indicates underload.
// func computePValue(in, out, freeInflight int, limit int) float64 {
// 	if out == 0 {
// 		return float64(limit)
// 	}
// 	return float64(in-(out+freeInflight)) / float64(out)
// }
