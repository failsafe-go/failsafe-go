package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var _ RateLimiterBuilder[any] = &rateLimiterConfig[any]{}

// Asserts that the smooth rate limiter factory methods are equal.
func TestShouldBuildEqualSmoothLimiters(t *testing.T) {
	interval1 := SmoothBuilder[any](10, time.Second).(*rateLimiterConfig[any]).interval
	interval2 := SmoothBuilderForMaxRate[any](100 * time.Millisecond).(*rateLimiterConfig[any]).interval

	assert.Equal(t, interval1, interval2)
}
