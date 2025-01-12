package adaptivelimiterold

// // pidExecutor is a policy.Executor that handles failures according to a PIDLimiter.
// type pidExecutor[R any] struct {
// 	*policy.BaseExecutor[R]
// 	*pidLimiter[R]
// }
//
// var _ policy.Executor[any] = &pidExecutor[any]{}
//
// func (e *pidExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
// 	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
// 		if permit, err := e.AcquirePermit(exec.Context()); err != nil {
// 			return internal.FailureResult[R](err)
// 		} else {
// 			execInternal := exec.(policy.ExecutionInternal[R])
// 			result := innerFn(exec)
// 			result = e.PostExecute(execInternal, result)
// 			permit.Record()
// 			return result
// 		}
// 	}
// }
