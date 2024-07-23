package cachepolicy

import (
	"context"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
)

// cacheExecutor is a policy.Executor that handles failures according to a CachePolicy.
type cacheExecutor[R any] struct {
	*policy.BaseExecutor[CachePolicyBuilder[R], R]
	*cachePolicy[R]
}

var _ policy.Executor[any] = &cacheExecutor[any]{}

func (e *cacheExecutor[R]) PreExecute(exec policy.ExecutionInternal[R]) *common.PolicyResult[R] {
	execInternal := exec.(policy.ExecutionInternal[R])
	if cacheKey := e.getCacheKey(exec.Context()); cacheKey != "" {
		if cacheResult, found := e.config.cache.Get(cacheKey); found {
			if e.config.onHit != nil {
				e.config.onHit(failsafe.ExecutionDoneEvent[R]{
					ExecutionInfo: execInternal,
					Result:        cacheResult,
				})
			}
			return &common.PolicyResult[R]{
				Result:     cacheResult,
				Done:       true,
				Success:    true,
				SuccessAll: true,
			}
		}
	}
	if e.config.onMiss != nil {
		e.config.onMiss(failsafe.ExecutionEvent[R]{
			ExecutionAttempt: execInternal,
		})
	}
	return nil
}

func (e *cacheExecutor[R]) PostExecute(exec policy.ExecutionInternal[R], er *common.PolicyResult[R]) *common.PolicyResult[R] {
	shouldCache := (len(e.config.cacheConditions) == 0 && er.Error == nil) ||
		util.AppliesToAny(e.config.cacheConditions, er.Result, er.Error)

	if shouldCache {
		if cacheKey := e.getCacheKey(exec.Context()); cacheKey != "" {
			e.config.cache.Set(cacheKey, er.Result)
			if e.config.onCache != nil {
				e.config.onCache(failsafe.ExecutionEvent[R]{
					ExecutionAttempt: exec.CopyWithResult(er),
				})
			}
		}
	}
	return er
}

func (e *cacheExecutor[R]) getCacheKey(ctx context.Context) string {
	if untypedKey := ctx.Value(CacheKey); untypedKey != nil {
		if typedKey, ok := untypedKey.(string); ok {
			return typedKey
		}
	}
	return e.config.key
}
