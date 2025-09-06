package testutil

import (
	"context"
	"reflect"
	"sync/atomic"
	"time"
	"unsafe"
)

var CanceledContextFn = func() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func ContextFn(ctx context.Context) func() context.Context {
	return func() context.Context {
		return ctx
	}
}

// ContextWithCancel returns a function that provides a context that is canceled after the sleepTime.
func ContextWithCancel(sleepTime time.Duration) func() context.Context {
	return func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(sleepTime)
			cancel()
		}()
		return ctx
	}
}

type TestClock struct {
	CurrentTime int64
}

func (t *TestClock) CurrentUnixNano() int64 {
	return t.CurrentTime
}

type TestStopwatch struct {
	CurrentTime int64
}

func (t *TestStopwatch) ElapsedTime() time.Duration {
	return time.Duration(t.CurrentTime)
}

func (t *TestStopwatch) Reset() {
	t.CurrentTime = 0
}

func Timed(fn func()) time.Duration {
	startTime := time.Now()
	fn()
	return time.Since(startTime)
}

type Waiter struct {
	count atomic.Int32
	done  chan struct{}
}

func NewWaiter() *Waiter {
	return &Waiter{
		done: make(chan struct{}, 10),
	}
}

func (w *Waiter) Await(expectedResumes int) {
	w.AwaitWithTimeout(expectedResumes, 0)
}

func (w *Waiter) AwaitWithTimeout(expectedResumes int, timeout time.Duration) {
	remainingResumes := w.count.Add(int32(expectedResumes))
	if remainingResumes != 0 {
		timer := time.NewTimer(timeout)
		select {
		case <-timer.C:
			panic("Timed out while waiting for a resume")
		case <-w.done:
			timer.Stop()
			return
		}
	}
}

func (w *Waiter) Resume() {
	remainingResumes := w.count.Add(int32(-1))
	if remainingResumes == 0 {
		w.done <- struct{}{}
	}
}

func MillisToNanos(millis int) int64 {
	return (time.Duration(millis) * time.Millisecond).Nanoseconds()
}

func GetPrioritizerRejectionThreshold(prioritizer any) *atomic.Int32 {
	val := reflect.ValueOf(prioritizer).Elem()
	field := val.FieldByName("RejectionThresh")
	if !field.IsValid() {
		panic("Failed to reflect RejectionThresh")
	}
	ptr := unsafe.Pointer(field.UnsafeAddr())
	return (*atomic.Int32)(ptr)
}
