package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func AssertDuration(t *testing.T, expectedDuration int, actualDuration time.Duration) {
	assert.Equal(t, time.Duration(expectedDuration), actualDuration)
}

func (w *Waiter) AssertEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return assert.Equal(t, expected, actual, msgAndArgs)
}
