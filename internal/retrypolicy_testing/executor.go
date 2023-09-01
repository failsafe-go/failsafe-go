package rptesting

import (
	"fmt"

	"failsafe"
	testutil "failsafe/internal/common_testing"
	"failsafe/retrypolicy"
)

func WithRetryStats[R any](rp retrypolicy.RetryPolicyBuilder[R], stats *testutil.Stats) retrypolicy.RetryPolicyBuilder[R] {
	return WithRetryStatsAndLogs(rp, stats, false)
}

func WithRetryStatsAndLogs[R any](rp retrypolicy.RetryPolicyBuilder[R], stats *testutil.Stats, withLogging bool) retrypolicy.RetryPolicyBuilder[R] {
	rp.OnFailedAttempt(func(e failsafe.ExecutionAttemptedEvent[R]) {
		stats.ExecutionCount++
		stats.FailedAttemptCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("RetryPolicy %s failed attempt [Result: %v, failure: %s, attempts: %d, executions: %d]",
				rp, e.LastResult, e.LastErr, e.Attempts, e.Executions))
		}
	}).OnRetry(func(e failsafe.ExecutionAttemptedEvent[R]) {
		stats.RetryCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("RetryPolicy %s retrying [Result: %v, failure: %s]", rp, e.LastResult, e.LastErr))
		}
	}).OnRetriesExceeded(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.RetrieExceededCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("RetryPolicy %s retries exceeded", rp))
		}
	}).OnAbort(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.AbortCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("RetryPolicy %s abort", rp))
		}
	})
	testutil.WithStatsAndLogs[retrypolicy.RetryPolicyBuilder[R], R](rp, stats, withLogging)
	return rp
}
