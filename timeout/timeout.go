package timeout

import (
	"errors"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrTimeoutExceeded is returned when an execution exceeds a configured timeout.
var ErrTimeoutExceeded = errors.New("timeout exceeded")

type Timeout[R any] interface {
	failsafe.Policy[R]
}

type TimeoutBuilder[R any] interface {
	// OnTimeoutExceeded registers the listener to be called when the timeout is exceeded.
	OnTimeoutExceeded(listener func(event failsafe.ExecutionCompletedEvent[R])) TimeoutBuilder[R]

	// Build returns a new Timeout using the builder's configuration.
	Build() Timeout[R]
}

type timeoutConfig[R any] struct {
	timeoutDelay      time.Duration
	onTimeoutExceeded func(failsafe.ExecutionCompletedEvent[R])
}

var _ TimeoutBuilder[any] = &timeoutConfig[any]{}

type timeout[R any] struct {
	config *timeoutConfig[R]
}

// With returns a new Timeout for execution result type R and the timeoutDelay.
func With[R any](timeoutDelay time.Duration) Timeout[R] {
	return Builder[R](timeoutDelay).Build()
}

// Builder returns a TimeoutBuilder for execution result type R which builds Timeouts for the timeoutDelay.
func Builder[R any](timeoutDelay time.Duration) TimeoutBuilder[R] {
	return &timeoutConfig[R]{
		timeoutDelay: timeoutDelay,
	}
}

func (c *timeoutConfig[R]) OnTimeoutExceeded(listener func(event failsafe.ExecutionCompletedEvent[R])) TimeoutBuilder[R] {
	c.onTimeoutExceeded = listener
	return c
}

func (c *timeoutConfig[R]) Build() Timeout[R] {
	fbCopy := *c
	return &timeout[R]{
		config: &fbCopy, // TODO copy base fields
	}
}

func (t *timeout[R]) ToExecutor(policyIndex int, _ R) any {
	te := &timeoutExecutor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{
			PolicyIndex: policyIndex,
		},
		timeout: t,
	}
	te.Executor = te
	return te
}
