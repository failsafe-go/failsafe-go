package timeout

import (
	"errors"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrExceeded is returned when an execution exceeds a configured timeout.
var ErrExceeded = errors.New("timeout exceeded")

// Timeout is a Policy that cancels executions if they exceed a time limit. Any policies composed inside the timeout,
// such as retries, will also be canceled. If the execution is configured with a Context, a child context will be created
// for the execution and canceled when the Timeout is exceeded.
//
// This type is concurrency safe.
type Timeout[R any] interface {
	failsafe.Policy[R]
}

type TimeoutBuilder[R any] interface {
	// OnTimeoutExceeded registers the listener to be called when the timeout is exceeded.
	OnTimeoutExceeded(listener func(event failsafe.ExecutionDoneEvent[R])) TimeoutBuilder[R]

	// Build returns a new Timeout using the builder's configuration.
	Build() Timeout[R]
}

type timeoutConfig[R any] struct {
	timeLimit         time.Duration
	onTimeoutExceeded func(failsafe.ExecutionDoneEvent[R])
}

var _ TimeoutBuilder[any] = &timeoutConfig[any]{}

type timeout[R any] struct {
	config *timeoutConfig[R]
}

// With returns a new Timeout for execution result type R and the timeLimit. The Timeout will cancel executions if they
// exceed a time limit. Any policies composed inside the timeout, such as retries, will also be canceled. If the
// execution is configured with a Context, a child context will be created for the execution and canceled when the
// Timeout is exceeded.
func With[R any](timeLimit time.Duration) Timeout[R] {
	return Builder[R](timeLimit).Build()
}

// Builder returns a TimeoutBuilder for execution result type R which builds Timeouts for the timeLimit. The Timeout will
// cancel executions if they exceed a time limit. Any policies composed inside the timeout, such as retries, will also be
// canceled. If the execution is configured with a Context, a child context will be created for the execution and canceled when the Timeout
// is exceeded.
func Builder[R any](timeLimit time.Duration) TimeoutBuilder[R] {
	return &timeoutConfig[R]{
		timeLimit: timeLimit,
	}
}

func (c *timeoutConfig[R]) OnTimeoutExceeded(listener func(event failsafe.ExecutionDoneEvent[R])) TimeoutBuilder[R] {
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
