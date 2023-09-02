package testutil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"failsafe"
)

type WhenGet[R any] func(execution failsafe.Execution[R]) (R, error)
type WhenRun[R any] func(execution failsafe.Execution[R]) error

func TestGetSuccess[R any](t *testing.T, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R) {
	testGet(t, executor, when, expectedAttempts, expectedExecutions, expectedResult, nil)
}

func TestGetFailure[R any](t *testing.T, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedError error) {
	testGet(t, executor, when, expectedAttempts, expectedExecutions, *(new(R)), expectedError)
}

func TestRunSuccess[R any](t *testing.T, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int) {
	testRun(t, executor, when, expectedAttempts, expectedExecutions, nil)
}

func TestRunFailure[R any](t *testing.T, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, expectedError error) {
	testRun(t, executor, when, expectedAttempts, expectedExecutions, expectedError)
}

func testGet[R any](t *testing.T, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R, expectedError error) {
	var completedEvent *failsafe.ExecutionCompletedEvent[R]
	result, err := executor.OnComplete(func(e failsafe.ExecutionCompletedEvent[R]) {
		completedEvent = &e
	}).GetWithExecution(when)
	assert.Equal(t, expectedAttempts, completedEvent.Attempts)
	assert.Equal(t, expectedExecutions, completedEvent.Executions)
	assert.Equal(t, expectedResult, result)
	assert.ErrorIs(t, expectedError, err)
}

func testRun[R any](t *testing.T, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, expectedError error) {
	var completedEvent *failsafe.ExecutionCompletedEvent[R]
	err := executor.OnComplete(func(e failsafe.ExecutionCompletedEvent[R]) {
		completedEvent = &e
	}).RunWithExecution(when)
	assert.Equal(t, expectedAttempts, completedEvent.Attempts)
	assert.Equal(t, expectedExecutions, completedEvent.Executions)
	assert.ErrorIs(t, expectedError, err)
}

type Stats struct {
	ExecutionCount      int
	FailedAttemptCount  int
	SuccessCount        int
	FailureCount        int
	RetryCount          int
	RetrieExceededCount int
	AbortCount          int
}

func (s *Stats) Reset() {
	s.ExecutionCount = 0
	s.FailedAttemptCount = 0
	s.SuccessCount = 0
	s.FailureCount = 0
	s.RetryCount = 0
	s.RetrieExceededCount = 0
	s.AbortCount = 0
}

func WithStatsAndLogs[P any, R any](policy failsafe.ListenablePolicyBuilder[P, R], stats *Stats, withLogging bool) {
	policy.OnSuccess(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.ExecutionCount++
		stats.SuccessCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("%s success [Result: %v, attempts: %d, executions: %d]",
				GetType(policy), e.Result, e.Attempts, e.Executions))
		}
	})
	policy.OnFailure(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.ExecutionCount++
		stats.FailureCount++
		if withLogging {
			fmt.Println(fmt.Sprintf("%s failure [Result: %v, failure: %s, attempts: %d, executions: %d]",
				GetType(policy), e.Result, e.Err, e.Attempts, e.Executions))
		}
	})
}
