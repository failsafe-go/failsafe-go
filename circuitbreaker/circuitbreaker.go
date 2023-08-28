package circuitbreaker

import (
	"errors"
	"sync"
	"time"

	"failsafe"
	"failsafe/internal/util"
)

var ErrCircuitBreakerOpen = errors.New("circuit breaker open")

type State int

const (
	ClosedState State = iota
	OpenState
	HalfOpenState
)

type CircuitBreaker[R any] interface {
	failsafe.Policy[R]

	Open()
	Close()
	HalfOpen()
	IsClosed() bool
	IsOpen() bool
	IsHalfOpen() bool
	GetState() State

	TryAcquirePermit() bool
	RecordResult(result R)
	RecordError(err error)
	RecordSuccess()
	RecordFailure()

	GetExecutionCount() uint
	GetRemainingDelay() time.Duration
	GetFailureCount() uint
	GetFailureRate() uint
	GetSuccessCount() uint
	GetSuccessRate() uint
}

type StateChangedEvent struct {
	PreviousState State
}

type circuitBreakerConfig[R any] struct {
	*failsafe.BaseListenablePolicy[R]
	*failsafe.BaseFailurePolicy[R]
	*failsafe.BaseDelayablePolicy[R]
	clock            util.Clock
	openListener     func(StateChangedEvent)
	halfOpenListener func(StateChangedEvent)
	closeListener    func(StateChangedEvent)

	failureThresholdConfig ThresholdConfig
	successThresholdConfig ThresholdConfig

	// Success config
	successThreshold            uint
	successThresholdingCapacity uint
}

var _ CircuitBreakerBuilder[any] = &circuitBreakerConfig[any]{}

type ThresholdConfig struct {
	threshold            uint
	rateThreshold        uint
	thresholdingCapacity uint
	executionThreshold   uint
	thresholdingPeriod   time.Duration
}

type circuitBreaker[R any] struct {
	config *circuitBreakerConfig[R]
	mtx    sync.Mutex
	// Guarded by mtx
	state circuitState[R]
}

var _ CircuitBreaker[any] = &circuitBreaker[any]{}

func (cb *circuitBreaker[R]) ToExecutor() failsafe.PolicyExecutor[R] {
	rpe := circuitBreakerExecutor[R]{
		BasePolicyExecutor: &failsafe.BasePolicyExecutor[R]{
			BaseListenablePolicy: cb.config.BaseListenablePolicy,
			BaseFailurePolicy:    cb.config.BaseFailurePolicy,
		},
		circuitBreaker: cb,
	}
	rpe.PolicyExecutor = &rpe
	return &rpe
}

func (cb *circuitBreaker[R]) TryAcquirePermit() bool {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	return cb.tryAcquirePermit()
}

func (cb *circuitBreaker[R]) Open() {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	cb.open(nil)
}

func (cb *circuitBreaker[R]) Close() {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	cb.close()
}

func (cb *circuitBreaker[R]) HalfOpen() {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	cb.halfOpen()
}

func (cb *circuitBreaker[R]) GetState() State {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	return cb.state.getState()
}

func (cb *circuitBreaker[R]) IsClosed() bool {
	return cb.GetState() == ClosedState
}

func (cb *circuitBreaker[R]) IsOpen() bool {
	return cb.GetState() == OpenState
}

func (cb *circuitBreaker[R]) IsHalfOpen() bool {
	return cb.GetState() == HalfOpenState
}

func (cb *circuitBreaker[R]) GetExecutionCount() uint {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	return cb.state.getStats().getExecutionCount()
}

func (cb *circuitBreaker[R]) GetRemainingDelay() time.Duration {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	return cb.state.getRemainingDelay()
}

func (cb *circuitBreaker[R]) GetFailureCount() uint {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	return cb.state.getStats().getFailureCount()
}

func (cb *circuitBreaker[R]) GetFailureRate() uint {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	return cb.state.getStats().getFailureRate()
}

func (cb *circuitBreaker[R]) GetSuccessCount() uint {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	return cb.state.getStats().getSuccessCount()
}

func (cb *circuitBreaker[R]) GetSuccessRate() uint {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	return cb.state.getStats().getSuccessRate()
}

func (cb *circuitBreaker[R]) RecordFailure() {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	cb.recordFailure(nil)
}

func (cb *circuitBreaker[R]) RecordError(err error) {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	cb.recordResult(*(new(R)), err)
}

func (cb *circuitBreaker[R]) RecordResult(result R) {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	cb.recordResult(result, nil)
}

func (cb *circuitBreaker[R]) RecordSuccess() {
	cb.mtx.Lock()
	defer cb.mtx.Unlock()
	cb.recordSuccess()
}

// Requires locking externally
func (cb *circuitBreaker[R]) transitionTo(newState State, exec *failsafe.Execution[R], listener func(StateChangedEvent)) {
	transitioned := false
	currentState := cb.state.getState()
	if cb.GetState() != newState {
		switch newState {
		case ClosedState:
			cb.state = newClosedState(cb)
		case OpenState:
			delay := cb.config.ComputeDelay(exec)
			if delay == -1 {
				delay = cb.config.Delay
			}
			cb.state = newOpenState(cb, cb.state, delay)
		case HalfOpenState:
			cb.state = newHalfOpenState(cb)
		}
		transitioned = true
	}

	if transitioned && listener != nil {
		listener(StateChangedEvent{
			PreviousState: currentState,
		})
	}
}

// Requires external locking
func (cb *circuitBreaker[R]) tryAcquirePermit() bool {
	return cb.state.tryAcquirePermit()
}

// Requires external locking
func (cb *circuitBreaker[R]) open(exec *failsafe.Execution[R]) {
	cb.transitionTo(OpenState, exec, cb.config.openListener)
}

// Requires external locking
func (cb *circuitBreaker[R]) close() {
	cb.transitionTo(ClosedState, nil, cb.config.closeListener)
}

// Requires external locking
func (cb *circuitBreaker[R]) halfOpen() {
	cb.transitionTo(HalfOpenState, nil, cb.config.halfOpenListener)
}

// Requires external locking
func (cb *circuitBreaker[R]) recordResult(result R, err error) {
	if cb.config.IsFailure(result, err) {
		cb.recordFailure(nil)
	} else {
		cb.recordSuccess()
	}
}

// Requires external locking
func (cb *circuitBreaker[R]) recordSuccess() {
	cb.state.getStats().recordSuccess()
	cb.state.checkThresholdAndReleasePermit(nil)
}

// Requires external locking
func (cb *circuitBreaker[R]) recordFailure(exec *failsafe.Execution[R]) {
	cb.state.getStats().recordFailure()
	cb.state.checkThresholdAndReleasePermit(exec)
}
