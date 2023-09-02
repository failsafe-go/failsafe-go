package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func AssertDuration(t *testing.T, expectedDuration int, actualDuration time.Duration) {
	assert.Equal(t, time.Duration(expectedDuration), actualDuration)
}
