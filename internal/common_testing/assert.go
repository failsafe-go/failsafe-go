package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type InvalidStateError struct {
	Cause error
}

func (ce InvalidStateError) Error() string {
	return "InvalidStateError"
}

func (ce InvalidStateError) Unwrap() error {
	return ce.Cause
}

type InvalidArgumentError struct {
	Cause error
}

func (ce InvalidArgumentError) Error() string {
	return "InvalidArgumentError"
}

func (ce InvalidArgumentError) Unwrap() error {
	return ce.Cause
}

type ConnectionError struct {
	Cause error
}

func (ce ConnectionError) Error() string {
	return "ConnectionError"
}
func (ce ConnectionError) Unwrap() error {
	return ce.Cause
}

type TimeoutError struct {
	Cause error
}

func (ce TimeoutError) Error() string {
	return "TimeoutError"
}

func (ce TimeoutError) Unwrap() error {
	return ce.Cause
}

func AssertDuration(t *testing.T, expectedDuration int, actualDuration time.Duration) {
	assert.Equal(t, time.Duration(expectedDuration), actualDuration)
}
