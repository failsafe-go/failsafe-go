package cachepolicy

import (
	"context"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
)

// executor is a policy.Executor that handles failures according to a CachePolicy.
type executor[R any] struct {
	policy.BaseExecutor[R]
	*cachePolicy[R]
}

var _ policy.Executor[any] = &executor[any]{}

func (e *executor[R]) PreExecute(exec policy.ExecutionInternal[R]) *common.PolicyResult[R] {
	if cacheKey := e.getCacheKey(exec.Context()); cacheKey != "" {
		if cacheResult, found := e.cache.Get(cacheKey); found {
			if e.onHit != nil {
				e.onHit(failsafe.ExecutionDoneEvent[R]{
					ExecutionInfo: exec,
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
	if e.onMiss != nil {
		e.onMiss(failsafe.ExecutionEvent[R]{
			ExecutionAttempt: exec,
		})
	}
	return nil
}

func (e *executor[R]) PostExecute(exec policy.ExecutionInternal[R], er *common.PolicyResult[R]) *common.PolicyResult[R] {
	shouldCache := (len(e.cacheConditions) == 0 && er.Error == nil) ||
		util.AppliesToAny(e.cacheConditions, er.Result, er.Error)

	if shouldCache {
		if cacheKey := e.getCacheKey(exec.Context()); cacheKey != "" {
			e.cache.Set(cacheKey, er.Result)
			if e.onCache != nil {
				e.onCache(failsafe.ExecutionEvent[R]{
					ExecutionAttempt: exec.CopyWithResult(er),
				})
			}
		}
	}
	return er
}

func (e *executor[R]) getCacheKey(ctx context.Context) string {
	if untypedKey := ctx.Value(CacheKey); untypedKey != nil {
		if typedKey, ok := untypedKey.(string); ok {
			return typedKey
		}
	}
	return e.key
}
