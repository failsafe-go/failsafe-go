package circuitbreaker

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/spi"
)

// circuitBreakerExecutor is a failsafe.PolicyExecutor that handles failures according to a CircuitBreaker.
type circuitBreakerExecutor[R any] struct {
	*spi.BasePolicyExecutor[R]
	*circuitBreaker[R]
}

var _ failsafe.PolicyExecutor[any] = &circuitBreakerExecutor[any]{}

func (cbe *circuitBreakerExecutor[R]) PreExecute(_ *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
	if !cbe.circuitBreaker.TryAcquirePermit() {
		return internal.FailureResult[R](ErrCircuitBreakerOpen)
	}
	return nil
}

func (cbe *circuitBreakerExecutor[R]) OnSuccess(exec *failsafe.ExecutionInternal[R], result *failsafe.ExecutionResult[R]) {
	cbe.BasePolicyExecutor.OnSuccess(exec, result)
	cbe.RecordSuccess()
}

func (cbe *circuitBreakerExecutor[R]) OnFailure(exec *failsafe.ExecutionInternal[R], result *failsafe.ExecutionResult[R]) *failsafe.ExecutionResult[R] {
	cbe.BasePolicyExecutor.OnFailure(exec, result)
	cbe.mtx.Lock()
	defer cbe.mtx.Unlock()
	cbe.recordFailure(&exec.Execution)
	return result
}
