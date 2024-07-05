package failsafegrpc

import (
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var retryableStatusCodes = map[codes.Code]struct{}{
	codes.Unavailable:       {},
	codes.DeadlineExceeded:  {},
	codes.ResourceExhausted: {},
}

// UnaryCallRetryPolicyBuilder returns a retrypolicy.RetryPolicyBuilder that will retry on
// gRPC status codes that are considered retryable(UNAVAILABLE, DEADLINE_EXCEEDED, RESOURCE_EXHAUSTED)
// up to 2 times by default.
// Additional handling can be added by chaining the builder with more conditions.
func UnaryCallRetryPolicyBuilder() retrypolicy.RetryPolicyBuilder[*UnaryClientResponse] {
	return retrypolicy.Builder[*UnaryClientResponse]().
		HandleIf(isRetryable[*UnaryClientResponse])
}

// StreamCallRetryPolicyBuilder returns a retrypolicy.RetryPolicyBuilder that will retry on
// gRPC status codes that are considered retryable(UNAVAILABLE, DEADLINE_EXCEEDED, RESOURCE_EXHAUSTED)
// up to 2 times by default.
func StreamCallRetryPolicyBuilder() retrypolicy.RetryPolicyBuilder[*StreamClientResponse] {
	return retrypolicy.Builder[*StreamClientResponse]().
		HandleIf(isRetryable[*StreamClientResponse])
}

func isRetryable[R any](_ R, err error) bool {
	if err != nil {
		s, ok := status.FromError(err)
		if !ok {
			return false
		}

		if _, ok := retryableStatusCodes[s.Code()]; ok {
			return true
		}
	}

	return false
}
