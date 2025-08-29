package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var _ Builder[any] = &config[any]{}

// Asserts that the smooth rate limiter factory methods are equal.
func TestShouldBuildEqualSmoothLimiters(t *testing.T) {
	interval1 := NewSmoothBuilder[any](10, time.Second).(*config[any]).interval
	interval2 := NewSmoothBuilderWithMaxRate[any](100 * time.Millisecond).(*config[any]).interval

	assert.Equal(t, interval1, interval2)
}
