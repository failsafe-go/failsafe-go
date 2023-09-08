package testutil

import (
	"reflect"
	"sync/atomic"
	"time"
)

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

func Timed(fn func()) time.Duration {
	startTime := time.Now()
	fn()
	return time.Now().Sub(startTime)
}

type Waiter struct {
	count atomic.Int32
	done  chan struct{}
}

func NewWaiter() *Waiter {
	return &Waiter{
		done: make(chan struct{}),
	}
}

func (w *Waiter) Await(expectedResumes int) {
	w.AwaitWithTimeout(expectedResumes, 0)
}

func (w *Waiter) AwaitWithTimeout(expectedResumes int, timeout time.Duration) {
	w.count.Add(int32(expectedResumes))
	timer := time.NewTimer(timeout)
	select {
	case <-timer.C:
		panic("Timed out while waiting for a resume")
	case <-w.done:
		timer.Stop()
		return
	}
}

func (w *Waiter) Resume() {
	remainingResumes := w.count.Add(int32(-1))
	if remainingResumes == 0 {
		close(w.done)
	}
	if remainingResumes < 0 {
		panic("too many Resume() calls")
	}
}

func MillisToNanos(millis int) int64 {
	return (time.Duration(millis) * time.Millisecond).Nanoseconds()
}

func GetType(myvar interface{}) string {
	if t := reflect.TypeOf(myvar); t.Kind() == reflect.Ptr {
		return t.Elem().Name()
	} else {
		return t.Name()
	}
}
