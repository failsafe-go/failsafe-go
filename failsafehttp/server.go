package failsafehttp

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/adaptivelimiter"
	"github.com/failsafe-go/failsafe-go/adaptivethrottler"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/priority"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/timeout"
)

// NewHandler returns a new http.Handler that will perform failsafe request handling via the policies and
// innerHandler. The policies are composed around responses and will handle responses in reverse order.
func NewHandler(innerHandler http.Handler, policies ...failsafe.Policy[*http.Response]) http.Handler {
	return NewHandlerWithExecutor(innerHandler, failsafe.With(policies...))
}

// NewHandlerWithExecutor returns a new http.Handler that will perform failsafe request handling via the executor and
// innerHandler. The policies are composed around responses and will handle responses in reverse order.
func NewHandlerWithExecutor(innerHandler http.Handler, executor failsafe.Executor[*http.Response]) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := executor.RunWithExecution(func(exec failsafe.Execution[*http.Response]) error {
			mergedCtx, cancel := util.MergeContexts(r.Context(), exec.Context())
			defer cancel(nil)
			innerHandler.ServeHTTP(w, r.WithContext(mergedCtx))
			return nil
		})

		if err != nil {
			var code int
			if errors.Is(err, bulkhead.ErrFull) ||
				errors.Is(err, circuitbreaker.ErrOpen) ||
				errors.Is(err, ratelimiter.ErrExceeded) ||
				errors.Is(err, adaptivelimiter.ErrExceeded) ||
				errors.Is(err, adaptivethrottler.ErrExceeded) {
				code = http.StatusTooManyRequests
			} else if errors.Is(err, timeout.ErrExceeded) {
				code = http.StatusServiceUnavailable
			} else {
				code = http.StatusInternalServerError
			}
			http.Error(w, err.Error(), code)
		}
	})
}

// NewHandlerWithLevel extracts adaptivelimiter priority and level information from an incoming request
// and adds a level to the handling context. If a level is present in the incoming request header, it's added to the
// context. If a level is not present but a priority is, and ensureLevel is true, then the priority will be converted
// to a level, else if a priority is present it will be passed through the context.
func NewHandlerWithLevel(innerHandler http.Handler, ensureLevel bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if levelStr := r.Header.Get(levelHeaderKey); levelStr != "" {
			if level, err := strconv.Atoi(levelStr); err == nil {
				ctx = priority.ContextWithLevel(ctx, level)
				r = r.WithContext(ctx)
			}
		} else if priorityStr := r.Header.Get(priorityHeaderKey); priorityStr != "" {
			if priorityInt, err := strconv.Atoi(priorityStr); err == nil {
				p := priority.Priority(priorityInt)
				if ensureLevel {
					ctx = priority.ContextWithLevel(ctx, p.RandomLevel())
				} else {
					ctx = p.AddTo(ctx)
				}
				r = r.WithContext(ctx)
			}
		}

		innerHandler.ServeHTTP(w, r)
	})
}
