package budget

import (
	"errors"
	"sync/atomic"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrExceeded is returned when an execution attempt exceeds the budget.
var ErrExceeded = errors.New("budget exceeded")

// Budget is a policy that restricts concurrent executions as a way of preventing system overload.
//
// R is the execution result type. This type is concurrency safe.
type Budget[R any] interface {
	failsafe.Policy[R]

	// AcquireRetryPermit acquires a permit to retry an execution, else returns ErrExceeded if the budget is exceeded.
	AcquireRetryPermit() error

	// AcquireHedgePermit acquires a permit to perform a hedged execution, else returns ErrExceeded if the budget is exceeded.
	AcquireHedgePermit() error

	// TryAcquireRetryPermit attempts to acquire a permit to retry an execution and returns whether it was successful.
	TryAcquireRetryPermit() bool

	// TryAcquireHedgePermit acquires a permit to perform a hedged execution and returns whether it was successful.
	TryAcquireHedgePermit() bool

	// ReleaseRetryPermit releases a previously acquired retry permit back to the budget.
	ReleaseRetryPermit()

	// ReleaseHedgePermit releases a previously acquired hedge permit back to the budget.
	ReleaseHedgePermit()
}

// TypeBuilder selects the execution type to apply budgeting to.
//
// R is the execution result type. This type is not concurrency safe.
type TypeBuilder[R any] interface {
	// ForRetries configures the budget to be applied to retries.
	ForRetries() Builder[R]

	// ForHedges configures the budget to be applied to hedges.
	ForHedges() Builder[R]
}

// Builder builds Budget instances.
//
// R is the execution result type. This type is not concurrency safe.
type Builder[R any] interface {
	TypeBuilder[R]

	// WithMaxRate configures the max rate of inflight executions that can be retries and/or hedges.
	WithMaxRate(maxRate float64) Builder[R]

	// WithMinConcurrency configures the min number of budgeted retries and/or hedges that can be executed, regardless of
	// the total number of inflight executions.
	WithMinConcurrency(minConcurrency uint) Builder[R]

	// OnBudgetExceeded registers the listener to be called when the budget is exceeded.
	OnBudgetExceeded(listener func(failsafe.ExecutionEvent[R])) Builder[R]

	// Build returns a new Budget using the builder's configuration.
	Build() Budget[R]
}

type config[R any] struct {
	forRetries       bool
	forHedges        bool
	maxRate          float64
	minConcurrency   uint
	onBudgetExceeded func(failsafe.ExecutionEvent[R])
}

var _ Builder[any] = &config[any]{}

// NewBuilder returns a TypeBuilder for execution result type R which builds Budgets with a default maxRate of .2 and
// minConcurrency of 3.
func NewBuilder[R any]() TypeBuilder[R] {
	return &config[R]{
		maxRate:        .2,
		minConcurrency: 3,
	}
}

func (c *config[R]) ForRetries() Builder[R] {
	c.forRetries = true
	return c
}

func (c *config[R]) ForHedges() Builder[R] {
	c.forHedges = true
	return c
}

func (c *config[R]) WithMaxRate(maxRate float64) Builder[R] {
	c.maxRate = maxRate
	return c
}

func (c *config[R]) WithMinConcurrency(minConcurrency uint) Builder[R] {
	c.minConcurrency = minConcurrency
	return c
}

func (c *config[R]) OnBudgetExceeded(listener func(failsafe.ExecutionEvent[R])) Builder[R] {
	c.onBudgetExceeded = listener
	return c
}

func (c *config[R]) Build() Budget[R] {
	return &budget[R]{
		config: *c, // TODO copy base fields
	}
}

type budget[R any] struct {
	config[R]

	executions atomic.Int32
	retries    atomic.Int32
	hedges     atomic.Int32
}

func (b *budget[R]) AcquireRetryPermit() error {
	rate := float64(b.retries.Load()) / float64(b.executions.Load())
	if rate > b.maxRate {
		return ErrExceeded
	}

	b.retries.Add(1)
	b.executions.Add(1)
	return nil
}

func (b *budget[R]) AcquireHedgePermit() error {
	rate := float64(b.hedges.Load()) / float64(b.executions.Load())
	if rate > b.maxRate {
		return ErrExceeded
	}

	b.retries.Add(1)
	b.executions.Add(1)
	return nil
}

func (b *budget[R]) TryAcquireRetryPermit() bool {
	return b.AcquireRetryPermit() == nil
}

func (b *budget[R]) TryAcquireHedgePermit() bool {
	return b.AcquireHedgePermit() == nil
}

func (b *budget[R]) ReleaseRetryPermit() {
	b.retries.Add(-1)
	b.executions.Add(-1)
}

func (b *budget[R]) ReleaseHedgePermit() {
	b.hedges.Add(-1)
	b.executions.Add(-1)
}

func (b *budget[R]) ToExecutor(_ R) any {
	be := &executor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{},
		budget:       b,
	}
	be.Executor = be
	return be
}
