package failsafe

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/failsafe-go/failsafe-go/common"
)

// ExecutionStats contains execution stats.
type ExecutionStats interface {
	// Attempts returns the number of execution attempts so far, including attempts that are currently in progress and
	// attempts that were blocked before being executed, such as by a CircuitBreaker or RateLimiter. These can include an initial
	// execution along with retries and hedges.
	Attempts() int

	// Executions returns the number of completed executions. Executions that are blocked, such as when a CircuitBreaker is
	// open, are not counted.
	Executions() int

	// Retries returns the number of retries so far, including retries that are currently in progress.
	Retries() int

	// Hedges returns the number of hedges that have been executed so far, including hedges that are currently in progress.
	Hedges() int

	// StartTime returns the time that the initial execution attempt started at.
	StartTime() time.Time

	// ElapsedTime returns the elapsed time since initial execution attempt began.
	ElapsedTime() time.Duration
}

// ExecutionAttempt contains information for an execution attempt.
type ExecutionAttempt[R any] interface {
	ExecutionStats

	// LastResult returns the result, if any, from the last execution attempt.
	LastResult() R

	// LastError returns the error, if any, from the last execution attempt.
	LastError() error

	// IsFirstAttempt returns true when Attempts is 1, meaning this is the first execution attempt.
	IsFirstAttempt() bool

	// IsRetry returns true when Attempts is > 1, meaning the execution is being retried.
	IsRetry() bool

	// IsHedge returns true when the execution is part of a hedged attempt.
	IsHedge() bool

	// AttemptStartTime returns the time that the most recent execution attempt started at.
	AttemptStartTime() time.Time

	// ElapsedAttemptTime returns the elapsed time since the last execution attempt began.
	ElapsedAttemptTime() time.Duration
}

// Execution contains information about an execution.
type Execution[R any] interface {
	ExecutionAttempt[R]

	// Context returns the context configured for the execution, else nil if none is configured. For executions that may
	// timeout, each attempt will get a separate child context.
	Context() context.Context

	// IsCanceled returns whether the execution has been canceled by an external Context or a timeout.Timeout.
	IsCanceled() bool

	// Canceled returns a channel that is closed when the execution is canceled, either by an external Context or a
	// timeout.Timeout.
	Canceled() <-chan any
}

// A closed channel that can be used as a canceled channel where the canceled channel would have been closed before it
// was accessed.
var closedChan chan any

func init() {
	closedChan = make(chan any, 1)
	close(closedChan)
}

type execution[R any] struct {
	// Shared state across instances
	mtx        *sync.Mutex
	startTime  time.Time
	ctx        context.Context
	attempts   *atomic.Uint32
	retries    *atomic.Uint32
	hedges     *atomic.Uint32
	executions *atomic.Uint32

	// Shared state, except for hedges
	canceledIndex  *int
	canceledResult **common.PolicyResult[R]
	canceled       *chan any

	// Per execution state
	attemptStartTime time.Time
	isHedge          bool
	lastResult       R     // The last error that occurred, else the zero value for R.
	lastError        error // The last error that occurred, else nil.
}

var _ Execution[any] = &execution[any]{}
var _ ExecutionStats = &execution[any]{}

func (e *execution[R]) Attempts() int {
	return int(e.attempts.Load())
}

func (e *execution[R]) Executions() int {
	return int(e.executions.Load())
}

func (e *execution[R]) Retries() int {
	return int(e.retries.Load())
}

func (e *execution[R]) Hedges() int {
	return int(e.hedges.Load())
}

func (e *execution[R]) StartTime() time.Time {
	return e.startTime
}

func (e *execution[R]) IsFirstAttempt() bool {
	return e.attempts.Load() == 1
}

func (e *execution[R]) IsRetry() bool {
	return e.attempts.Load() > 1
}

func (e *execution[R]) IsHedge() bool {
	return e.isHedge
}

func (e *execution[R]) ElapsedTime() time.Duration {
	return time.Since(e.startTime)
}

func (e *execution[R]) LastResult() R {
	return e.lastResult
}

func (e *execution[R]) LastError() error {
	return e.lastError
}

func (e *execution[R]) Context() context.Context {
	return e.ctx
}

func (e *execution[R]) AttemptStartTime() time.Time {
	return e.attemptStartTime
}

func (e *execution[_]) ElapsedAttemptTime() time.Duration {
	return time.Since(e.attemptStartTime)
}

func (e *execution[_]) IsCanceled() bool {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	return *e.canceledIndex > -1
}

func (e *execution[_]) Canceled() <-chan any {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	// Create channel lazily
	if *e.canceled == nil {
		*e.canceled = make(chan any, 1)
	}
	return *e.canceled
}

func (e *execution[R]) InitializeRetry(policyIndex int, lastResult *common.PolicyResult[R]) *common.PolicyResult[R] {
	// Lock to guard against a race with a Timeout canceling the execution
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if canceled, cancelResult := e.isCanceledForPolicy(policyIndex); canceled {
		return cancelResult
	}

	if e.attempts.Add(1) > 1 {
		e.retries.Add(1)
	}
	e.attemptStartTime = time.Now()
	*e.canceledIndex = -1
	*e.canceledResult = nil
	*e.canceled = nil
	if lastResult != nil {
		e.lastResult = lastResult.Result
		e.lastError = lastResult.Error
	}
	return nil
}

func (e *execution[R]) InitializeHedge(policyIndex int) *common.PolicyResult[R] {
	// Lock to guard against a race with a Timeout canceling the execution
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if canceled, cancelResult := e.isCanceledForPolicy(policyIndex); canceled {
		return cancelResult
	}

	e.isHedge = true
	e.attempts.Add(1)
	e.hedges.Add(1)
	return nil
}

func (e *execution[R]) Cancel(policyIndex int, result *common.PolicyResult[R]) {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if e.isCanceled() {
		return
	}
	*e.canceledIndex = policyIndex
	*e.canceledResult = result
	if result != nil {
		e.lastResult = result.Result
		e.lastError = result.Error
	}
	if *e.canceled != nil {
		close(*e.canceled)
	} else {
		*e.canceled = closedChan
	}
}

// Requires locking externally.
func (e *execution[R]) isCanceled() bool {
	return *e.canceledIndex != -1
}

func (e *execution[R]) IsCanceledForPolicy(policyIndex int) (bool, *common.PolicyResult[R]) {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	return e.isCanceledForPolicy(policyIndex)
}

// Requires locking externally.
func (e *execution[R]) isCanceledForPolicy(policyIndex int) (bool, *common.PolicyResult[R]) {
	if *e.canceledIndex >= policyIndex && *e.canceledIndex != -1 {
		return true, *e.canceledResult
	}
	return false, nil
}

func (e *execution[R]) CopyWithResult(result *common.PolicyResult[R]) Execution[R] {
	e.mtx.Lock()
	c := *e
	e.mtx.Unlock()
	if result != nil {
		c.lastResult = result.Result
		c.lastError = result.Error
	}
	return &c
}

func (e *execution[R]) CopyWithContext(ctx context.Context) Execution[R] {
	e.mtx.Lock()
	c := *e
	e.mtx.Unlock()
	c.ctx = ctx
	return &c
}

func (e *execution[R]) copy() *execution[R] {
	e.mtx.Lock()
	c := *e
	e.mtx.Unlock()
	return &c
}

func (e *execution[R]) record() {
	e.executions.Add(1)
}

func newExecution[R any](ctx context.Context) *execution[R] {
	attempts := atomic.Uint32{}
	retries := atomic.Uint32{}
	hedges := atomic.Uint32{}
	executions := atomic.Uint32{}
	attempts.Add(1)
	canceledIndex := -1
	var canceledResult *common.PolicyResult[R]
	var canceled chan any
	now := time.Now()
	return &execution[R]{
		ctx:              ctx,
		mtx:              &sync.Mutex{},
		attempts:         &attempts,
		retries:          &retries,
		hedges:           &hedges,
		executions:       &executions,
		canceledIndex:    &canceledIndex,
		canceledResult:   &canceledResult,
		canceled:         &canceled,
		attemptStartTime: now,
		startTime:        now,
	}
}
