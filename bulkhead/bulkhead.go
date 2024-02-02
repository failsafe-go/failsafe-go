package bulkhead

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrFull is returned when an execution is attempted against a Bulkhead that is full.
var ErrFull = errors.New("bulkhead full")

// Bulkhead is a policy restricts concurrent executions as a way of preventing system overload.
//
// This type is concurrency safe.
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
// This type is not concurrency safe.
type BulkheadBuilder[R any] interface {
	// WithMaxWaitTime configures the maxWaitTime to wait for permits to be available.
	WithMaxWaitTime(maxWaitTime time.Duration) BulkheadBuilder[R]

	// OnBulkheadFull registers the listener to be called when the bulkhead is full.
	OnBulkheadFull(listener func(event failsafe.ExecutionEvent[R])) BulkheadBuilder[R]

	// Build returns a new Bulkhead using the builder's configuration.
	Build() Bulkhead[R]
}

type bulkheadConfig[R any] struct {
	maxConcurrency uint
	maxWaitTime    time.Duration
	onBulkheadFull func(failsafe.ExecutionEvent[R])
}

func (c *bulkheadConfig[R]) WithMaxWaitTime(maxWaitTime time.Duration) BulkheadBuilder[R] {
	c.maxWaitTime = maxWaitTime
	return c
}

func (c *bulkheadConfig[R]) OnBulkheadFull(listener func(event failsafe.ExecutionEvent[R])) BulkheadBuilder[R] {
	c.onBulkheadFull = listener
	return c
}

func (c *bulkheadConfig[R]) Build() Bulkhead[R] {
	return &bulkhead[R]{
		config:    c, // TODO copy base fields
		semaphore: semaphore.NewWeighted(int64(c.maxConcurrency)),
	}
}

var _ BulkheadBuilder[any] = &bulkheadConfig[any]{}

// With returns a new Bulkhead for execution result type R and the maxConcurrency.
func With[R any](maxConcurrency uint) Bulkhead[R] {
	return Builder[R](maxConcurrency).Build()
}

// Builder returns a BulkheadBuilder for execution result type R which builds Timeouts for the timeoutDelay.
func Builder[R any](maxConcurrency uint) BulkheadBuilder[R] {
	return &bulkheadConfig[R]{
		maxConcurrency: maxConcurrency,
	}
}

type bulkhead[R any] struct {
	config    *bulkheadConfig[R]
	semaphore *semaphore.Weighted
}

func (b *bulkhead[R]) AcquirePermit(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := b.semaphore.Acquire(ctx, 1); err != nil {
		return ErrFull
	}
	return nil
}

func (b *bulkhead[R]) AcquirePermitWithMaxWait(ctx context.Context, maxWaitTime time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, maxWaitTime)
	err := b.semaphore.Acquire(ctx, 1)
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		err = ErrFull
	}
	cancel()
	return err
}

func (b *bulkhead[R]) TryAcquirePermit() bool {
	return b.semaphore.TryAcquire(1)
}

func (b *bulkhead[R]) ReleasePermit() {
	b.semaphore.Release(1)
}

func (b *bulkhead[R]) ToExecutor(policyIndex int, _ R) any {
	be := &bulkheadExecutor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{
			PolicyIndex: policyIndex,
		},
		bulkhead: b,
	}
	be.Executor = be
	return be
}
