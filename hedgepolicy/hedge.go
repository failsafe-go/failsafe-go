package hedgepolicy

import (
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// HedgePolicy is a policy that performes additional executions if the initial execution is slow to complete. This policy
// differs from RetryPolicy since multiple hedged execution may be in progress at the same time. The first execution to
// be done is returned and the remaining executions are canceled. Once the max hedges have been started, they are left to
// run until one completes, then the remaining executions are canceled.
//
// This type is concurrency safe.
type HedgePolicy[R any] interface {
	failsafe.Policy[R]
}

// HedgePolicyBuilder builds HedgePolicy instances.
//
// This type is not concurrency safe.
type HedgePolicyBuilder[R any] interface {
	// OnHedge registers the listener to be called when a hedge is about to be attempted.
	OnHedge(listener func(failsafe.ExecutionEvent[R])) HedgePolicyBuilder[R]

	// WithMaxHedges sets the max number of hedges to perform when an execution attempt doesn't complete in time, which is 1
	// by default.
	WithMaxHedges(maxHedges int) HedgePolicyBuilder[R]

	// Build returns a new HedgePolicy using the builder's configuration.
	Build() HedgePolicy[R]
}

type hedgePolicyConfig[R any] struct {
	delayFunc failsafe.DelayFunc[R]
	maxHedges int
	onHedge   func(failsafe.ExecutionEvent[R])
}

var _ HedgePolicyBuilder[any] = &hedgePolicyConfig[any]{}

// WithDelay returns a new HedgePolicy for execution result type R and the delay, which by default will allow a single
// hedged execution to be performed, after the delay is elapsed, if the original execution is not done yet. Additional
// hedged executions will be performed, with delay, up to the max configured hedges.
func WithDelay[R any](delay time.Duration) HedgePolicy[R] {
	return BuilderWithDelay[R](delay).Build()
}

// WithDelayFunc returns a new HedgePolicy for execution result type R and the delayFunc, which by default will allow a
// single hedged execution to be performed, after the delayFunc result is elapsed, if the original execution is not done
// yet. Additional hedged executions will be performed, with delay, up to the max configured hedges.
func WithDelayFunc[R any](delayFunc failsafe.DelayFunc[R]) HedgePolicy[R] {
	return BuilderWithDelayFunc[R](delayFunc).Build()
}

// BuilderWithDelay returns a new HedgePolicyBuilder for execution result type R and the delay, which by default will
// allow a single hedged execution to be performed, after the delay is elapsed, if the original execution is not done
// yet. Additional hedged executions will be performed, with delay, up to the max configured hedges.
func BuilderWithDelay[R any](delay time.Duration) HedgePolicyBuilder[R] {
	return BuilderWithDelayFunc[R](func(exec failsafe.ExecutionAttempt[R]) time.Duration {
		return delay
	})
}

// BuilderWithDelayFunc returns a new HedgePolicyBuilder for execution result type R and the delayFunc, which by default
// will allow a single hedged execution to be performed, after the delayFunc result is elapsed, if the original execution
// is not done yet. Additional hedged executions will be performed, with delay, up to the max configured hedges.
func BuilderWithDelayFunc[R any](delayFunc failsafe.DelayFunc[R]) HedgePolicyBuilder[R] {
	return &hedgePolicyConfig[R]{
		delayFunc: delayFunc,
		maxHedges: 1,
	}
}

type hedgePolicy[R any] struct {
	config *hedgePolicyConfig[R]
}

var _ HedgePolicy[any] = &hedgePolicy[any]{}

func (c *hedgePolicyConfig[R]) OnHedge(listener func(failsafe.ExecutionEvent[R])) HedgePolicyBuilder[R] {
	c.onHedge = listener
	return c
}

func (c *hedgePolicyConfig[R]) WithMaxHedges(maxHedges int) HedgePolicyBuilder[R] {
	c.maxHedges = maxHedges
	return c
}

func (c *hedgePolicyConfig[R]) Build() HedgePolicy[R] {
	hCopy := *c
	return &hedgePolicy[R]{
		config: &hCopy, // TODO copy base fields
	}
}

func (h *hedgePolicy[R]) ToExecutor(policyIndex int, _ R) any {
	he := &hedgeExecutor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{
			PolicyIndex: policyIndex,
		},
		hedgePolicy: h,
	}
	he.Executor = he
	return he
}
