package circuitbreaker

import (
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// circuitBreakerExecutor is a failsafe.PolicyExecutor that handles failures according to a CircuitBreaker.
type circuitBreakerExecutor[R any] struct {
	*policy.BasePolicyExecutor[R]
	*circuitBreaker[R]
}

var _ policy.PolicyExecutor[any] = &circuitBreakerExecutor[any]{}

func (cbe *circuitBreakerExecutor[R]) PreExecute(_ policy.ExecutionInternal[R]) *common.ExecutionResult[R] {
	if !cbe.circuitBreaker.TryAcquirePermit() {
		return internal.FailureResult[R](ErrCircuitBreakerOpen)
	}
	return nil
}

func (cbe *circuitBreakerExecutor[R]) OnSuccess(exec policy.ExecutionInternal[R], result *common.ExecutionResult[R]) {
	cbe.BasePolicyExecutor.OnSuccess(exec, result)
	cbe.RecordSuccess()
}

func (cbe *circuitBreakerExecutor[R]) OnFailure(exec policy.ExecutionInternal[R], result *common.ExecutionResult[R]) *common.ExecutionResult[R] {
	cbe.BasePolicyExecutor.OnFailure(exec, result)
	cbe.mtx.Lock()
	defer cbe.mtx.Unlock()
	cbe.recordFailure(exec)
	return result
}
