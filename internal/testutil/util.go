package testutil

import (
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe"
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

type Waiter struct {
	count int32
	done  chan struct{}
}

func NewWaiter() *Waiter {
	return &Waiter{
		done: make(chan struct{}),
	}
}

func (w *Waiter) AssertEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return assert.Equal(t, expected, actual, msgAndArgs)
}

func (w *Waiter) Await(expectedResumes int) {
	w.AwaitWithTimeout(expectedResumes, 0)
}

func (w *Waiter) AwaitWithTimeout(expectedResumes int, timeout time.Duration) {
	atomic.AddInt32(&w.count, int32(expectedResumes))
	select {
	case <-w.done:
		return
	case <-time.After(timeout):
		panic("Timed out while waiting for a resume")
	}
}

func (w *Waiter) Resume() {
	i := atomic.AddInt32(&w.count, -1)
	if i == 0 {
		close(w.done)
	}
	if i < 0 {
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

func ErrorNTimesThenReturn[R any](err error, errorTimes int, result R) func(exec failsafe.Execution[R]) (R, error) {
	counter := 0
	return func(exec failsafe.Execution[R]) (R, error) {
		if counter < errorTimes {
			counter++
			defaultResult := *(new(R))
			return defaultResult, err
		}
		return result, nil
	}
}
