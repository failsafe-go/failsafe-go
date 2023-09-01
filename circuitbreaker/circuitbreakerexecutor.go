package circuitbreaker

import (
	"failsafe"
)

// circuitBreakerExecutor is a failsafe.PolicyExecutor that handles failures according to a CircuitBreaker.
type circuitBreakerExecutor[R any] struct {
	*failsafe.BasePolicyExecutor[R]
	*circuitBreaker[R]
}

var _ failsafe.PolicyExecutor[any] = &circuitBreakerExecutor[any]{}

func (cbe *circuitBreakerExecutor[R]) PreExecute() *failsafe.ExecutionResult[R] {
	if !cbe.circuitBreaker.TryAcquirePermit() {
		return &failsafe.ExecutionResult[R]{
			Err: ErrCircuitBreakerOpen,
		}
	}
	return nil
}

func (cbe *circuitBreakerExecutor[R]) OnSuccess(_ *failsafe.ExecutionResult[R]) {
	cbe.RecordSuccess()
}

func (cbe *circuitBreakerExecutor[R]) OnFailure(exec *failsafe.Execution[R], result *failsafe.ExecutionResult[R]) *failsafe.ExecutionResult[R] {
	cbe.mtx.Lock()
	defer cbe.mtx.Unlock()
	cbe.recordFailure(exec)
	return result
}
