package failsafe

import (
	"context"
)

/*
Executor handles failures according to configured policies. An executor can be created for specific policies via:

	failsafe.With(outerPolicy, policies)
*/
type Executor[R any] interface {
	// Compose returns a new Executor that composes the currently configured policies around the given innerPolicy. For example, consider:
	//
	//     failsafe.With(fallback).Compose(retryPolicy).Compose(circuitBreaker)
	//
	// This results in the following internal composition when executing a func and handling its result:
	//
	//     Fallback(RetryPolicy(CircuitBreaker(func)))
	Compose(innerPolicy Policy[R]) Executor[R]

	// WithContext configures a ctx that can be used to cancel executions.
	WithContext(ctx context.Context) Executor[R]

	// OnComplete registers the listener to be called when an execution is complete.
	OnComplete(listener func(ExecutionCompletedEvent[R])) Executor[R]

	// OnSuccess registers the listener to be called when an execution is successful. If multiple policies, are configured, this handler is
	// called when execution is complete and all policies succeed. If all policies do not succeed, then the OnFailure registered listener is
	// called instead.
	OnSuccess(listener func(ExecutionCompletedEvent[R])) Executor[R]

	// OnFailure registers the listener to be called when an execution fails. This occurs when the execution fails according to some policy,
	// and all policies have been exceeded.
	OnFailure(listener func(ExecutionCompletedEvent[R])) Executor[R]

	// Run executes the runnable until successful or until the configured policies are exceeded.
	Run(fn func() error) (err error)

	// RunWithExecution executes the runnable until successful or until the configured policies are exceeded, while providing an Execution
	// to the fn.
	RunWithExecution(fn func(exec Execution[R]) error) (err error)

	// Get executes the runnable until a successful result is returned or the configured policies are exceeded.
	Get(fn func() (R, error)) (R, error)

	// GetWithExecution executes the runnable until a successful result is returned or the configured policies are exceeded, while providing
	// an Execution to the fn.
	GetWithExecution(fn func(exec Execution[R]) (R, error)) (R, error)
}

type executor[R any] struct {
	policies   []Policy[R]
	ctx        context.Context
	onComplete func(ExecutionCompletedEvent[R])
	onSuccess  func(ExecutionCompletedEvent[R])
	onFailure  func(ExecutionCompletedEvent[R])
}

/*
With creates and returns a new Executor for result type R that will handle failures according to the given policies. The policies are
composed around an execution and will handle execution results in reverse, with the last policy being applied first. For example, consider:

	failsafe.With(fallback, retryPolicy, circuitBreaker).Get(func)

This is equivalent to composition using the Compose method:

	failsafe.With(fallback).Compose(retryPolicy).Compose(circuitBreaker).Get(func)

These result in the following internal composition when executing a func and handling its result:

	Fallback(RetryPolicy(CircuitBreaker(func)))
*/
func With[R any](outerPolicy Policy[R], policies ...Policy[R]) Executor[R] {
	policies = append([]Policy[R]{outerPolicy}, policies...)
	return &executor[R]{
		policies: policies,
	}
}

func (e *executor[R]) Compose(innerPolicy Policy[R]) Executor[R] {
	e.policies = append(e.policies, innerPolicy)
	return e
}

func (e *executor[R]) WithContext(ctx context.Context) Executor[R] {
	e.ctx = ctx
	return e
}

func (e *executor[R]) OnComplete(listener func(ExecutionCompletedEvent[R])) Executor[R] {
	e.onComplete = listener
	return e
}

func (e *executor[R]) OnSuccess(listener func(ExecutionCompletedEvent[R])) Executor[R] {
	e.onSuccess = listener
	return e
}

func (e *executor[R]) OnFailure(listener func(ExecutionCompletedEvent[R])) Executor[R] {
	e.onFailure = listener
	return e
}

func (e *executor[R]) Run(fn func() error) (err error) {
	_, err = e.GetWithExecution(func(exec Execution[R]) (R, error) {
		return *(new(R)), fn()
	})
	return err
}

func (e *executor[R]) RunWithExecution(fn func(exec Execution[R]) error) (err error) {
	_, err = e.GetWithExecution(func(exec Execution[R]) (R, error) {
		return *(new(R)), fn(exec)
	})
	return err
}

func (e *executor[R]) Get(fn func() (R, error)) (R, error) {
	return e.GetWithExecution(func(exec Execution[R]) (R, error) {
		return fn()
	})
}

func (e *executor[R]) GetWithExecution(fn func(exec Execution[R]) (R, error)) (R, error) {
	outerFn := func(execInternal *ExecutionInternal[R]) *ExecutionResult[R] {
		result, err := fn(execInternal.Execution)
		er := &ExecutionResult[R]{
			Result:     result,
			Err:        err,
			Complete:   true,
			Success:    true,
			SuccessAll: true,
		}
		execInternal.recordAttempt(er)
		return er
	}

	// Compose policy executors from the innermost policy to the outermost
	for i := len(e.policies) - 1; i >= 0; i-- {
		outerFn = e.policies[i].ToExecutor().Apply(outerFn)
	}

	execInternal := &ExecutionInternal[R]{
		Execution: Execution[R]{
			Context:        e.ctx,
			ExecutionStats: ExecutionStats{},
		},
	}
	execInternal.InitializeAttempt()
	er := outerFn(execInternal)
	if e.onSuccess != nil && er.SuccessAll {
		e.onSuccess(*newExecutionCompletedEvent(er, &execInternal.ExecutionStats))
	} else if e.onFailure != nil && !er.SuccessAll {
		e.onFailure(*newExecutionCompletedEvent(er, &execInternal.ExecutionStats))
	}
	if e.onComplete != nil {
		e.onComplete(*newExecutionCompletedEvent(er, &execInternal.ExecutionStats))
	}
	return er.Result, er.Err
}
