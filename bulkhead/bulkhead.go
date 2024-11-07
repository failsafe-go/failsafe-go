package bulkhead

import (
	"context"
	"errors"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrFull is returned when an execution is attempted against a Bulkhead that is full.
var ErrFull = errors.New("bulkhead full")

// Bulkhead is a policy restricts concurrent executions as a way of preventing system overload.
//
// R is the execution result type. This type is concurrency safe.
type Bulkhead[R any] interface {
	failsafe.Policy[R]

	// AcquirePermit attempts to acquire a permit to perform an execution against within the Bulkhead, waiting until one is
	// available or the execution is canceled. Returns context.Canceled if the ctx is canceled. Callers should call
	// ReleasePermit to release a successfully acquired permit back to the Bulkhead.
	//
	// ctx may be nil.
	AcquirePermit(ctx context.Context) error

	// AcquirePermitWithMaxWait attempts to acquire a permit to perform an execution within the Bulkhead, waiting up to the
	// maxWaitTime until one is available or the ctx is canceled. Returns ErrFull if a permit could not be acquired
	// in time. Returns context.Canceled if the ctx is canceled. Callers should call ReleasePermit to release a successfully
	// acquired permit back to the Bulkhead.
	//
	// ctx may be nil.
	AcquirePermitWithMaxWait(ctx context.Context, maxWaitTime time.Duration) error

	// ReleasePermit releases an execution permit back to the Bulkhead.
	ReleasePermit()

	// TryAcquirePermit tries to acquire a permit to perform an execution within the Bulkhead, returning immediately without
	// waiting. Returns true if the permit was acquired, else false. Callers should call ReleasePermit to release a
	// successfully acquired permit back to the Bulkhead.
	TryAcquirePermit() bool
}

// BulkheadBuilder builds Bulkhead instances.
//
// R is the execution result type. This type is not concurrency safe.
type BulkheadBuilder[R any] interface {
	// WithMaxWaitTime configures the maxWaitTime to wait for permits to be available.
	WithMaxWaitTime(maxWaitTime time.Duration) BulkheadBuilder[R]

	// OnFull registers the listener to be called when the bulkhead is full.
	OnFull(listener func(event failsafe.ExecutionEvent[R])) BulkheadBuilder[R]

	// Build returns a new Bulkhead using the builder's configuration.
	Build() Bulkhead[R]
}

type config[R any] struct {
	maxConcurrency uint
	maxWaitTime    time.Duration
	onFull         func(failsafe.ExecutionEvent[R])
}

func (c *config[R]) WithMaxWaitTime(maxWaitTime time.Duration) BulkheadBuilder[R] {
	c.maxWaitTime = maxWaitTime
	return c
}

func (c *config[R]) OnFull(listener func(event failsafe.ExecutionEvent[R])) BulkheadBuilder[R] {
	c.onFull = listener
	return c
}

func (c *config[R]) Build() Bulkhead[R] {
	return &bulkhead[R]{
		config:    c, // TODO copy base fields
		semaphore: make(chan struct{}, c.maxConcurrency),
	}
}

var _ BulkheadBuilder[any] = &config[any]{}

// With returns a new Bulkhead for execution result type R and the maxConcurrency.
func With[R any](maxConcurrency uint) Bulkhead[R] {
	return Builder[R](maxConcurrency).Build()
}

// Builder returns a BulkheadBuilder for execution result type R which builds Timeouts for the timeoutDelay.
func Builder[R any](maxConcurrency uint) BulkheadBuilder[R] {
	return &config[R]{
		maxConcurrency: maxConcurrency,
	}
}

type bulkhead[R any] struct {
	*config[R]
	semaphore chan struct{}
}

func (b *bulkhead[R]) AcquirePermit(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case b.semaphore <- struct{}{}:
		return nil
	}
}

func (b *bulkhead[R]) AcquirePermitWithMaxWait(ctx context.Context, maxWaitTime time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Initial attempt, in case permit is immediately available or context is done, so we don't race with a timer
	select {
	case <-ctx.Done():
		return ctx.Err()
	case b.semaphore <- struct{}{}:
		return nil
	default:
		if maxWaitTime == 0 {
			return ErrFull
		}
	}

	// Second attempt with timer
	timer := time.NewTimer(maxWaitTime)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case b.semaphore <- struct{}{}:
		return nil
	case <-timer.C:
		return ErrFull
	}
}

func (b *bulkhead[R]) TryAcquirePermit() bool {
	select {
	case b.semaphore <- struct{}{}:
		return true
	default:
		return false
	}
}

func (b *bulkhead[R]) ReleasePermit() {
	<-b.semaphore
}

func (b *bulkhead[R]) ToExecutor(_ R) any {
	be := &executor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{},
		bulkhead:     b,
	}
	be.Executor = be
	return be
}
