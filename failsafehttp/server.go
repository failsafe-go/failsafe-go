package failsafehttp

import (
	"errors"
	"net/http"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/timeout"
)

// NewHandler returns a new http.Handler that will perform failsafe request handling via the policies and
// innerHandler. The policies are composed around responses and will handle responses in reverse order.
func NewHandler(innerHandler http.Handler, policies ...failsafe.Policy[*http.Response]) http.Handler {
	return NewHandlerWithExecutor(innerHandler, failsafe.NewExecutor(policies...))
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
			if errors.Is(err, bulkhead.ErrFull) || errors.Is(err, circuitbreaker.ErrOpen) || errors.Is(err, ratelimiter.ErrExceeded) {
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
