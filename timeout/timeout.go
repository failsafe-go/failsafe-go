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
// R is the execution result type. This type is concurrency safe.
type Timeout[R any] interface {
	failsafe.Policy[R]
}

// Builder builds Timeout instances.
//
// R is the execution result type. This type is not concurrency safe.
type Builder[R any] interface {
	// OnTimeoutExceeded registers the listener to be called when the timeout is exceeded.
	OnTimeoutExceeded(listener func(event failsafe.ExecutionDoneEvent[R])) Builder[R]

	// Build returns a new Timeout using the builder's configuration.
	Build() Timeout[R]
}

type config[R any] struct {
	timeLimit         time.Duration
	onTimeoutExceeded func(failsafe.ExecutionDoneEvent[R])
}

var _ Builder[any] = &config[any]{}

// New returns a new Timeout for execution result type R and the timeLimit. The Timeout will cancel executions if they
// exceed a time limit. Any policies composed inside the timeout, such as retries, will also be canceled. If the
// execution is configured with a Context, a child context will be created for the execution and canceled when the
// Timeout is exceeded.
func New[R any](timeLimit time.Duration) Timeout[R] {
	return NewBuilder[R](timeLimit).Build()
}

// NewBuilder returns a Builder for execution result type R which builds Timeouts for the timeLimit. The Timeout will
// cancel executions if they exceed a time limit. Any policies composed inside the timeout, such as retries, will also be
// canceled. If the execution is configured with a Context, a child context will be created for the execution and canceled when the Timeout
// is exceeded.
func NewBuilder[R any](timeLimit time.Duration) Builder[R] {
	return &config[R]{
		timeLimit: timeLimit,
	}
}

func (c *config[R]) OnTimeoutExceeded(listener func(event failsafe.ExecutionDoneEvent[R])) Builder[R] {
	c.onTimeoutExceeded = listener
	return c
}

func (c *config[R]) Build() Timeout[R] {
	fbCopy := *c
	return &timeout[R]{
		config: &fbCopy, // TODO copy base fields
	}
}

type timeout[R any] struct {
	*config[R]
}

func (t *timeout[R]) ToExecutor(_ R) any {
	te := &executor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{},
		timeout:      t,
	}
	te.Executor = te
	return te
}
