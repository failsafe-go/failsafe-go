package timeout

import (
	"errors"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/spi"
)

// ErrTimeoutExceeded is returned when an execution exceeds a configured timeout.
var ErrTimeoutExceeded = errors.New("timeout exceeded")

type Timeout[R any] interface {
	failsafe.Policy[R]
}

type TimeoutBuilder[R any] interface {
	failsafe.ListenablePolicyBuilder[TimeoutBuilder[R], R]

	// Build returns a new Timeout using the builder's configuration.
	Build() Timeout[R]
}

type timeoutConfig[R any] struct {
	*spi.BaseListenablePolicy[R]
	timeoutDelay time.Duration
}

var _ TimeoutBuilder[any] = &timeoutConfig[any]{}

type timeout[R any] struct {
	config *timeoutConfig[R]
}

func With[R any](timeoutDelay time.Duration) Timeout[R] {
	return Builder[R](timeoutDelay).Build()
}

// Builder returns a TimeoutBuilder for execution result type R which builds Timeouts for the timeoutDelay.
func Builder[R any](timeoutDelay time.Duration) TimeoutBuilder[R] {
	return &timeoutConfig[R]{
		BaseListenablePolicy: &spi.BaseListenablePolicy[R]{},
		timeoutDelay:         timeoutDelay,
	}
}

func (c *timeoutConfig[R]) OnSuccess(listener func(event failsafe.ExecutionCompletedEvent[R])) TimeoutBuilder[R] {
	c.BaseListenablePolicy.OnSuccess(listener)
	return c
}

func (c *timeoutConfig[R]) OnFailure(listener func(event failsafe.ExecutionCompletedEvent[R])) TimeoutBuilder[R] {
	c.BaseListenablePolicy.OnFailure(listener)
	return c
}

func (c *timeoutConfig[R]) Build() Timeout[R] {
	fbCopy := *c
	return &timeout[R]{
		config: &fbCopy, // TODO copy base fields
	}
}

func (t *timeout[R]) ToExecutor(policyIndex int) failsafe.PolicyExecutor[R] {
	te := &timeoutExecutor[R]{
		BasePolicyExecutor: &spi.BasePolicyExecutor[R]{
			BaseListenablePolicy: t.config.BaseListenablePolicy,
			PolicyIndex:          policyIndex,
		},
		timeout: t,
	}
	te.PolicyExecutor = te
	return te
}
