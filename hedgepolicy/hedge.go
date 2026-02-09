package hedgepolicy

import (
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/budget"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/internal/util"
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

	// WithMaxHedges sets the max number of hedges to perform when an execution attempt doesn't complete in time, which is 1
	// by default.
	WithMaxHedges(maxHedges int) Builder[R]

	// WithBudget configures a hedge budget. When the hedgeBudget is exceeded, hedges will stop with budget.ErrExceeded.
	WithBudget(hedgeBudget budget.Budget) Builder[R]

	// OnHedge registers the listener to be called when a hedge is about to be attempted.
	OnHedge(listener func(failsafe.ExecutionEvent[R])) Builder[R]

	// Build returns a new HedgePolicy using the builder's configuration.
	Build() HedgePolicy[R]
}

type config[R any] struct {
	policy.BaseAbortablePolicy[R]

	delayFunc          failsafe.DelayFunc[R]
	maxHedges          int
	budget             internal.Budget
	onHedge            func(failsafe.ExecutionEvent[R])
	mu                 *sync.RWMutex
	quantile           *util.MovingQuantile // Guarded by mu
	quantileValue      float64              // quantile percentile (0-1), used to defer MovingQuantile creation to Build
	quantileAge        uint                 // age for the quantile's EMA decay
	executionThreshold uint                 // min executions before hedging begins
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

// NewBuilderWithDelayFunc returns a new Builder for execution result type R and the delayFunc, which by default will
// allow a single hedged execution to be performed, after the delayFunc result is elapsed, if the original execution is
// not done yet. Additional hedged executions will be performed, after additional delays, up to the max configured
// hedges.
//
// If the execution is configured with a Context, a child context will be created for the execution and canceled when
// the HedgePolicy is exceeded.
func NewBuilderWithDelayFunc[R any](delayFunc failsafe.DelayFunc[R]) Builder[R] {
	return &config[R]{
		BaseAbortablePolicy: policy.BaseAbortablePolicy[R]{},
		delayFunc:           delayFunc,
		maxHedges:           1,
	}
}

// NewWithDelayQuantile returns a new HedgePolicy for execution result type R that automatically determines the hedge
// delay based on recent execution durations, hedging when an execution exceeds the given quantile of observed
// durations. For example, a quantile of 0.95 will hedge executions that take longer than the p95 of recent successful
// execution times. The quantileAge controls how many executions the policy effectively "remembers" - smaller ages adapt
// faster to recent changes, while larger ages provide more stability. The executionThreshold is the minimum number of
// executions that must be recorded before hedging begins.
//
// Panics if quantile is not > 0 and < 1, or if quantileAge or executionThreshold are 0.
func NewWithDelayQuantile[R any](quantile float64, quantileAge uint, executionThreshold uint) HedgePolicy[R] {
	return NewBuilderWithDelayQuantile[R](quantile, quantileAge, executionThreshold).Build()
}

// NewBuilderWithDelayQuantile returns a new Builder for execution result type R that automatically determines the hedge
// delay based on recent execution durations, hedging when an execution exceeds the given quantile of observed
// durations. For example, a quantile of 0.95 will hedge executions that take longer than the p95 of recent successful
// execution times. The quantileAge controls how many executions the policy effectively "remembers" - smaller ages adapt
// faster to recent changes, while larger ages provide more stability. The executionThreshold is the minimum number of
// executions that must be recorded before hedging begins.
//
// Panics if quantile is not > 0 and < 1, or if quantileAge or executionThreshold are 0.
func NewBuilderWithDelayQuantile[R any](quantile float64, quantileAge uint, executionThreshold uint) Builder[R] {
	util.Assert(quantile > 0 && quantile < 1, "quantile must be between 0 and 1 exclusive")
	util.Assert(quantileAge > 0, "quantileAge must be > 0")
	util.Assert(executionThreshold > 0, "executionThreshold must be > 0")
	return &config[R]{
		BaseAbortablePolicy: policy.BaseAbortablePolicy[R]{},
		maxHedges:           1,
		quantileValue:       quantile,
		quantileAge:         quantileAge,
		executionThreshold:  executionThreshold,
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

func (c *config[R]) WithMaxHedges(maxHedges int) Builder[R] {
	c.maxHedges = maxHedges
	return c
}

func (c *config[R]) WithBudget(hedgeBudget budget.Budget) Builder[R] {
	c.budget = hedgeBudget.(internal.Budget)
	return c
}

func (c *config[R]) OnHedge(listener func(failsafe.ExecutionEvent[R])) Builder[R] {
	c.onHedge = listener
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

	// Initialize quantile-based delay
	if cCopy.quantileValue != 0 {
		mu := &sync.RWMutex{}
		mq := util.NewMovingQuantile(cCopy.quantileValue, 0.01, cCopy.quantileAge)
		cCopy.mu = mu
		cCopy.quantile = &mq
		executionThreshold := cCopy.executionThreshold
		cCopy.delayFunc = func(exec failsafe.ExecutionAttempt[R]) time.Duration {
			mu.RLock()
			defer mu.RUnlock()
			if mq.Count() < int(executionThreshold) {
				return -1
			}
			return time.Duration(mq.Value())
		}
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
		BaseExecutor: policy.BaseExecutor[R]{},
		hedgePolicy:  h,
	}
	he.Executor = he
	return he
}
