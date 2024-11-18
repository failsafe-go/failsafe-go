package adaptivelimiter

import (
	"context"
)

// Limiter allows execution permits to be acquired and limits executions based on successes, failures, and other configuration.
type Limiter interface {
	// AcquirePermit attempts to acquire a permit to perform an execution via the limiter, waiting until one is
	// available or the execution is canceled. Returns context.Canceled if the ctx is canceled.
	// Callers should call Record or Drop to release a successfully acquired permit back to the limiter.
	// ctx may be nil.
	AcquirePermit(context.Context) (Permit, error)

	// TryAcquirePermit attempts to acquire a permit to perform an execution via the limiter, returning whether the permit was acquired or not.
	// Callers should call Record or Drop to release a successfully acquired permit back to the limiter.
	TryAcquirePermit() (Permit, bool)
}

// Permit is a permit to perform an execution that must be completed by calling RecordSuccess, RecordFailure, or Release.
type Permit interface {
	// Record records an execution completion and releases a permit back to the limiter. The execution duration will be used
	// to influence the limiter.
	Record()

	// Drop releases an execution permit back to the limiter without recording a completion. This should be used when an
	// execution completes prematurely, such as via a timeout, and we don't want the execution duration to influence the
	// limiter.
	Drop()
}
