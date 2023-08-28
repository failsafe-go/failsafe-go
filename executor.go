package failsafe

import (
	"context"
)

type Executor[R any] struct {
	policies   []Policy[R]
	ctx        context.Context
	onComplete func(ExecutionCompletedEvent[R])
}

func With(policies ...Policy[any]) Executor[any] {
	return Executor[any]{
		policies: policies,
	}
}

func WithResult[R any](policies ...Policy[R]) Executor[R] {
	return Executor[R]{
		policies: policies,
	}
}

//func (e Executor[R]) With(policies ...Policy[R]) Executor[R] {
//	e.policies = policies
//	return e
//}

//func ForResult[R any]() Executor[R] {
//	return Executor[R]{}
//}

func (e Executor[R]) WithContext(ctx context.Context) Executor[R] {
	e.ctx = ctx
	return e
}

func (e Executor[R]) OnComplete(listener func(ExecutionCompletedEvent[R])) Executor[R] {
	e.onComplete = listener
	return e
}

func (e Executor[R]) Run(fn func() error) (err error) {
	_, err = e.GetWithExecution(func(exec Execution[R]) (R, error) {
		return *(new(R)), fn()
	})
	return err
}

func (e Executor[R]) RunWithExecution(fn func(exec Execution[R]) error) (err error) {
	_, err = e.GetWithExecution(func(exec Execution[R]) (R, error) {
		return *(new(R)), fn(exec)
	})
	return err
}

func (e Executor[R]) Get(fn func() (R, error)) (R, error) {
	return e.GetWithExecution(func(exec Execution[R]) (R, error) {
		return fn()
	})
}

func (e Executor[R]) GetWithExecution(fn func(exec Execution[R]) (R, error)) (R, error) {
	outerFn := func(execInternal *ExecutionInternal[R]) *ExecutionResult[R] {
		result, err := fn(execInternal.Execution)
		er := &ExecutionResult[R]{
			Result: result,
			Err:    err,
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
			ExecutionStats: ExecutionStats[R]{},
		},
	}

	execInternal.InitializeAttempt()
	er := outerFn(execInternal)
	if e.onComplete != nil {
		e.onComplete(ExecutionCompletedEvent[R]{
			Result:         er.Result,
			Err:            er.Err,
			ExecutionStats: execInternal.ExecutionStats,
		})
	}
	return er.Result, er.Err
}
