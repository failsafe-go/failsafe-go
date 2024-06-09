package failsafegrpc

import (
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"google.golang.org/grpc"
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
func UnaryCallRetryPolicyBuilder() retrypolicy.RetryPolicyBuilder[any] {
	retryHandleFunc := func(reply any, err error) bool {
		if err != nil {
			return isRetryable(err)
		}

		return false
	}

	return retrypolicy.Builder[any]().
		HandleIf(retryHandleFunc)
}

// StreamCallRetryPolicyBuilder returns a retrypolicy.RetryPolicyBuilder that will retry on
// gRPC status codes that are considered retryable(UNAVAILABLE, DEADLINE_EXCEEDED, RESOURCE_EXHAUSTED)
// up to 2 times by default.
func StreamCallRetryPolicyBuilder() retrypolicy.RetryPolicyBuilder[grpc.ClientStream] {
	retryHandleFunc := func(stream grpc.ClientStream, err error) bool {
		if err != nil {
			return isRetryable(err)
		}

		return false
	}

	return retrypolicy.Builder[grpc.ClientStream]().
		HandleIf(retryHandleFunc)
}

func isRetryable(err error) bool {
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
