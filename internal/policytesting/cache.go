package policytesting

import (
	"github.com/failsafe-go/failsafe-go/cachepolicy"
)

type TestCache[R any] struct {
	Cache map[string]R
}

func (c *TestCache[R]) Get(key string) (R, bool) {
	result, found := c.Cache[key]
	return result, found
}

func (c *TestCache[R]) Set(key string, value R) {
	c.Cache[key] = value
}

func NewCache[R any]() (map[string]R, cachepolicy.Cache[R]) {
	cache := make(map[string]R)
	return cache, &TestCache[R]{
		Cache: cache,
	}
}
