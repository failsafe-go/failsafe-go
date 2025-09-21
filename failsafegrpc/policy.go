package failsafegrpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// NewRetryPolicyBuilder returns a retrypolicy.Builder that will retry on gRPC status codes that are considered
// retryable (UNAVAILABLE, DEADLINE_EXCEEDED, RESOURCE_EXHAUSTED), up to 2 times by default, with no delay between
// attempts. Additional handling can be added by chaining the builder with more conditions.
//
// R is the execution result type.
func NewRetryPolicyBuilder[R any]() retrypolicy.Builder[R] {
	return retrypolicy.NewBuilder[R]().HandleIf(func(_ R, err error) bool {
		if err != nil {
			if s, ok := status.FromError(err); ok {
				switch s.Code() {
				case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
					return true
				}
			}
		}

		return false
	})
}
