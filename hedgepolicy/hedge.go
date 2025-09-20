package hedgepolicy

import (
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// HedgePolicy is a policy that performes additional execution attempts if the initial execution is slow to complete,
// with a delay between attempts. This policy differs from RetryPolicy since multiple hedged executions may be in
// progress at the same time. By default, any outstanding hedges are canceled after the first execution result or error
// returns. The CancelOn and CancelIf methods can be used to configure a hedge policy to cancel after different results,
// errors, or conditions. Once the max hedges have been started, they are left to run until a cancellable result is
// returned, then the remaining hedges are canceled.
//
// If the execution is configured with a Context, a child context will be created for each attempt and outstanding
// contexts are canceled when the HedgePolicy is exceeded.
//
// R is the execution result type. This type is concurrency safe.
type HedgePolicy[R any] interface {
	failsafe.Policy[R]
}

// Builder builds HedgePolicy instances.
//
// R is the execution result type. This type is not concurrency safe.
type Builder[R any] interface {
	// CancelOnResult specifies that any outstanding hedges should be canceled if the execution result matches the result using
	// reflect.DeepEqual.
	CancelOnResult(result R) Builder[R]

	// CancelOnErrors specifies that any outstanding hedges should be canceled if the execution error matches any of the errors
	// using errors.Is.
	CancelOnErrors(errs ...error) Builder[R]

	// CancelOnErrorTypes specifies the errors whose types should cause any outstanding hedges to be canceled. Any execution
	// errors or their Unwrapped parents whose type matches any of the errs' types will cause outstanding hedges to be
	// canceled. This is similar to the check that errors.As performs.
	CancelOnErrorTypes(errs ...any) Builder[R]

	// CancelIf specifies that any outstanding hedges should be canceled if the predicate matches the result or error.
	CancelIf(predicate func(R, error) bool) Builder[R]

	// OnHedge registers the listener to be called when a hedge is about to be attempted.
	OnHedge(listener func(failsafe.ExecutionEvent[R])) Builder[R]

	// WithMaxHedges sets the max number of hedges to perform when an execution attempt doesn't complete in time, which is 1
	// by default.
	WithMaxHedges(maxHedges int) Builder[R]

	// Build returns a new HedgePolicy using the builder's configuration.
	Build() HedgePolicy[R]
}

type config[R any] struct {
	policy.BaseAbortablePolicy[R]

	delayFunc failsafe.DelayFunc[R]
	maxHedges int
	onHedge   func(failsafe.ExecutionEvent[R])
}

var _ Builder[any] = &config[any]{}

// NewWithDelay returns a new HedgePolicy for execution result type R and the delay, which by default will allow a single
// hedged execution to be performed, after the delay is elapsed, if the original execution is not done yet. Additional
// hedged executions will be performed, with delay, up to the max configured hedges.
//
// If the execution is configured with a Context, a child context will be created for the execution and canceled when the
// HedgePolicy is exceeded.
func NewWithDelay[R any](delay time.Duration) HedgePolicy[R] {
	return NewBuilderWithDelay[R](delay).Build()
}

// NewWithDelayFunc returns a new HedgePolicy for execution result type R and the delayFunc, which by default will allow a
// single hedged execution to be performed, after the delayFunc result is elapsed, if the original execution is not done
// yet. Additional hedged executions will be performed, with delay, up to the max configured hedges.
//
// If the execution is configured with a Context, a child context will be created for the execution and canceled when the
// HedgePolicy is exceeded.
func NewWithDelayFunc[R any](delayFunc failsafe.DelayFunc[R]) HedgePolicy[R] {
	return NewBuilderWithDelayFunc[R](delayFunc).Build()
}

// NewBuilderWithDelay returns a new Builder for execution result type R and the delay, which by default will
// allow a single hedged execution to be performed, after the delay is elapsed, if the original execution is not done
// yet. Additional hedged executions will be performed, with delay, up to the max configured hedges.
//
// If the execution is configured with a Context, a child context will be created for the execution and canceled when the
// HedgePolicy is exceeded.
func NewBuilderWithDelay[R any](delay time.Duration) Builder[R] {
	return NewBuilderWithDelayFunc[R](func(exec failsafe.ExecutionAttempt[R]) time.Duration {
		return delay
	})
}

// NewBuilderWithDelayFunc returns a new Builder for execution result type R and the delayFunc, which by default
// will allow a single hedged execution to be performed, after the delayFunc result is elapsed, if the original execution
// is not done yet. Additional hedged executions will be performed, after additional delays, up to the max configured hedges.
//
// If the execution is configured with a Context, a child context will be created for the execution and canceled when the
// HedgePolicy is exceeded.
func NewBuilderWithDelayFunc[R any](delayFunc failsafe.DelayFunc[R]) Builder[R] {
	return &config[R]{
		BaseAbortablePolicy: policy.BaseAbortablePolicy[R]{},
		delayFunc:           delayFunc,
		maxHedges:           1,
	}
}

func (c *config[R]) CancelOnResult(result R) Builder[R] {
	c.BaseAbortablePolicy.AbortOnResult(result)
	return c
}

func (c *config[R]) CancelOnErrors(errs ...error) Builder[R] {
	c.BaseAbortablePolicy.AbortOnErrors(errs...)
	return c
}

func (c *config[R]) CancelOnErrorTypes(errs ...any) Builder[R] {
	c.BaseAbortablePolicy.AbortOnErrorTypes(errs...)
	return c
}

func (c *config[R]) CancelIf(predicate func(R, error) bool) Builder[R] {
	c.BaseAbortablePolicy.AbortIf(predicate)
	return c
}

func (c *config[R]) OnHedge(listener func(failsafe.ExecutionEvent[R])) Builder[R] {
	c.onHedge = listener
	return c
}

func (c *config[R]) WithMaxHedges(maxHedges int) Builder[R] {
	c.maxHedges = maxHedges
	return c
}

func (c *config[R]) Build() HedgePolicy[R] {
	cCopy := *c
	if !cCopy.BaseAbortablePolicy.IsConfigured() {
		// Cancel hedges by default after any result is received
		cCopy.AbortIf(func(r R, err error) bool {
			return true
		})
	}
	return &hedgePolicy[R]{
		config: cCopy, // TODO copy base fields
	}
}

type hedgePolicy[R any] struct {
	config[R]
}

var _ HedgePolicy[any] = &hedgePolicy[any]{}

func (h *hedgePolicy[R]) ToExecutor(_ R) any {
	he := &executor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{},
		hedgePolicy:  h,
	}
	he.Executor = he
	return he
}
