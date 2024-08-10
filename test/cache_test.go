package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/cachepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

// Tests adding and getting an item from the cache. Uses different cache key scenarios, including global, per context,
// and no cache key.
func TestCache(t *testing.T) {
	// Given
	cache, failsafeCache := policytesting.NewCache[string]()
	stats := &policytesting.Stats{}

	// When / Then
	tests := []struct {
		name               string
		executor           failsafe.Executor[string]
		expectedExecutions int
		expectedResult     string
		expectedCaches     int
		expectedHits       int
		expectedMisses     int
	}{
		{
			name: "with global key",
			executor: failsafe.NewExecutor[string](
				policytesting.WithCacheStats(cachepolicy.NewBuilder[string](failsafeCache).
					WithKey("foo"), stats).
					Build(),
			),
			expectedExecutions: 0,
			expectedResult:     "bar",
			expectedCaches:     1,
			expectedHits:       1,
			expectedMisses:     0,
		},
		{
			name: "with context key",
			executor: failsafe.NewExecutor[string](
				policytesting.WithCacheStats(cachepolicy.NewBuilder[string](failsafeCache), stats).Build()).
				WithContext(context.WithValue(context.Background(), cachepolicy.CacheKey, "foo2")),
			expectedExecutions: 0,
			expectedResult:     "bar",
			expectedCaches:     1,
			expectedHits:       1,
			expectedMisses:     0,
		},
		{
			name: "with no key",
			executor: failsafe.NewExecutor[string](
				policytesting.WithCacheStats(cachepolicy.NewBuilder[string](failsafeCache), stats).Build(),
			),
			expectedExecutions: 1,
			expectedResult:     "missing",
			expectedCaches:     0,
			expectedHits:       0,
			expectedMisses:     1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setup := func() {
				stats.Reset()
				clear(cache)
			}

			// Add item to cache
			testutil.Test[string](t).
				WithExecutor(tc.executor).
				Setup(setup).
				Get(testutil.GetFn("bar", nil)).
				AssertSuccess(1, 1, "bar", func() {
					assert.Equal(t, tc.expectedCaches, stats.Caches())
					assert.Equal(t, 0, stats.CacheHits())
					assert.Equal(t, 1, stats.CacheMisses())
				})

			// Get item from cache
			testutil.Test[string](t).
				WithExecutor(tc.executor).
				Reset(stats).
				Get(testutil.GetFn("missing", nil)).
				AssertSuccess(1, tc.expectedExecutions, tc.expectedResult, func() {
					assert.Equal(t, 0, stats.Caches())
					assert.Equal(t, tc.expectedHits, stats.CacheHits())
					assert.Equal(t, tc.expectedMisses, stats.CacheMisses())
				})
		})
	}
}

func TestConditionalCache(t *testing.T) {
	// Given
	cache, failsafeCache := policytesting.NewCache[string]()
	stats := &policytesting.Stats{}
	barPredicate := func(s string, err error) bool { return s == "bar" }

	// When / Then
	tests := []struct {
		name           string
		cpb            cachepolicy.Builder[string]
		result         string
		expectedCaches int
	}{
		{
			name: "with matching condition",
			cpb: policytesting.WithCacheStats(cachepolicy.NewBuilder[string](failsafeCache), stats).WithKey("foo").
				CacheIf(barPredicate),
			result:         "bar",
			expectedCaches: 1,
		},
		{
			name: "with non-matching condition",
			cpb: policytesting.WithCacheStats(cachepolicy.NewBuilder[string](failsafeCache), stats).WithKey("foo").
				CacheIf(barPredicate),
			result:         "baz",
			expectedCaches: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setup := func() {
				stats.Reset()
				clear(cache)
			}

			// When / Then
			testutil.Test[string](t).
				WithExecutor(failsafe.NewExecutor[string](tc.cpb.Build())).
				Setup(setup).
				Get(testutil.GetFn(tc.result, nil)).
				AssertSuccess(1, 1, tc.result, func() {
					assert.Equal(t, tc.expectedCaches, stats.Caches())
					assert.Equal(t, 0, stats.CacheHits())
					assert.Equal(t, 1, stats.CacheMisses())
				})
		})
	}
}

// Tests that a result is not cached when an error occurs.
func TestDoNotCacheOnError(t *testing.T) {
	// Given
	_, failsafeCache := policytesting.NewCache[string]()
	stats := &policytesting.Stats{}
	cp := policytesting.WithCacheStats(cachepolicy.NewBuilder[string](failsafeCache), stats).
		WithKey("foo").
		Build()

	// When / Then
	testutil.Test[string](t).
		With(cp).
		Reset(stats).
		Get(testutil.GetFn("", testutil.ErrInvalidState)).
		AssertSuccessError(1, 1, testutil.ErrInvalidState, func() {
			assert.Equal(t, 0, stats.Caches())
			assert.Equal(t, 0, stats.CacheHits())
			assert.Equal(t, 1, stats.CacheMisses())
		})
}
