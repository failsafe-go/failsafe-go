package cachepolicy

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

type key int

// CacheKey is a key to use with a Context that stores the cache key.
const CacheKey key = 0

// Cache is a simple interface for cached values that can be adapted to different cache backends.
//
// R is the execution result type.
type Cache[R any] interface {
	// Get gets and returns a cache entry along with a flag indicating if it's present.
	Get(key string) (R, bool)

	// Set stores a value for the key in the cache.
	Set(key string, value R)
}

// CachePolicy is a read through cache Policy that sets and gets cached results for some key. The cache key can be
// configured via CachePolicyBuilder, or by setting a CacheKey value in a Context used with an execution.
//
// R is the execution result type. This type is concurrency safe.
type CachePolicy[R any] interface {
	failsafe.Policy[R]
}

// CachePolicyBuilder builds CachePolicy instances. In order for the cache policy to be used, a key must be provided via
// WithKey, or via a Context when the execution is performed using a value stored under the CacheKey in the Context. A
// cache key stored in a Context takes precedence over a cache key configured via WithKey.
//
// R is the execution result type. This type is not concurrency safe.
type CachePolicyBuilder[R any] interface {
	// WithKey builds caches that store successful execution results in a cache with the key. This key can be overridden by
	// providing a CacheKey in a Context used with an execution.
	WithKey(key string) CachePolicyBuilder[R]

	// CacheIf specifies that a value result should only be cached if it satisfies the predicate. By default, any non-error
	// results will be cached.
	CacheIf(predicate func(R, error) bool) CachePolicyBuilder[R]

	// OnCacheHit registers the listener to be called when the cachePolicy entry is hit during an execution.
	OnCacheHit(listener func(event failsafe.ExecutionDoneEvent[R])) CachePolicyBuilder[R]

	// OnCacheMiss registers the listener to be called when the cachePolicy entry is missed during an execution.
	OnCacheMiss(listener func(event failsafe.ExecutionEvent[R])) CachePolicyBuilder[R]

	// OnResultCached registers the listener to be called when a result is cached.
	OnResultCached(listener func(event failsafe.ExecutionEvent[R])) CachePolicyBuilder[R]

	// Build returns a new CachePolicy using the builder's configuration.
	Build() CachePolicy[R]
}

type config[R any] struct {
	cache           Cache[R]
	key             string
	cacheConditions []func(result R, err error) bool
	onHit           func(event failsafe.ExecutionDoneEvent[R])
	onMiss          func(failsafe.ExecutionEvent[R])
	onCache         func(failsafe.ExecutionEvent[R])
}

var _ CachePolicyBuilder[any] = &config[any]{}

type cachePolicy[R any] struct {
	*config[R]
}

// With returns a new CachePolicy. The resulting CachePolicy will only be used with executions that provide a Context
// containing a CacheKey value.
func With[R any](cache Cache[R]) CachePolicy[R] {
	return Builder[R](cache).Build()
}

// Builder returns a CachePolicyBuilder.
func Builder[R any](cache Cache[R]) CachePolicyBuilder[R] {
	return &config[R]{
		cache: cache,
	}
}

func (c *config[R]) CacheIf(predicate func(R, error) bool) CachePolicyBuilder[R] {
	c.cacheConditions = append(c.cacheConditions, predicate)
	return c
}

func (c *config[R]) WithKey(key string) CachePolicyBuilder[R] {
	c.key = key
	return c
}

func (c *config[R]) OnCacheHit(listener func(event failsafe.ExecutionDoneEvent[R])) CachePolicyBuilder[R] {
	c.onHit = listener
	return c
}

func (c *config[R]) OnCacheMiss(listener func(event failsafe.ExecutionEvent[R])) CachePolicyBuilder[R] {
	c.onMiss = listener
	return c
}

func (c *config[R]) OnResultCached(listener func(event failsafe.ExecutionEvent[R])) CachePolicyBuilder[R] {
	c.onCache = listener
	return c
}

func (c *config[R]) Build() CachePolicy[R] {
	return &cachePolicy[R]{
		config: c, // TODO copy base fields
	}
}

func (c *cachePolicy[R]) ToExecutor(_ R) any {
	ce := &executor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{},
		cachePolicy:  c,
	}
	ce.Executor = ce
	return ce
}
