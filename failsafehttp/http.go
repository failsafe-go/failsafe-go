package failsafehttp

import (
	"net/http"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
)

type roundTripper struct {
	next     http.RoundTripper
	executor failsafe.Executor[*http.Response]
}

// NewRoundTripper returns a new http.RoundTripper that will perform failsafe round trips via the policies and
// innerRoundTripper. If innerRoundTripper is nil, http.DefaultTransport will be used. The policies are composed around
// requests and will handle responses in reverse order.
func NewRoundTripper(innerRoundTripper http.RoundTripper, policies ...failsafe.Policy[*http.Response]) http.RoundTripper {
	return NewRoundTripperWithExecutor(innerRoundTripper, failsafe.NewExecutor(policies...))
}

// NewRoundTripperWithExecutor returns a new http.RoundTripper that will perform failsafe round trips via the executor and
// innerRoundTripper. If innerRoundTripper is nil, http.DefaultTransport will be used.
func NewRoundTripperWithExecutor(innerRoundTripper http.RoundTripper, executor failsafe.Executor[*http.Response]) http.RoundTripper {
	if innerRoundTripper == nil {
		innerRoundTripper = http.DefaultTransport
	}
	return &roundTripper{
		next:     innerRoundTripper,
		executor: executor,
	}
}

func (f *roundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f.executor.GetWithExecution(func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
		ctx, cancel := util.MergeContexts(request.Context(), exec.Context())
		defer cancel()
		return f.next.RoundTrip(request.WithContext(ctx))
	})
}

type Request struct {
	executor failsafe.Executor[*http.Response]
	request  *http.Request
	client   *http.Client
}

// NewRequest creates and returns a new Request that will perform failsafe round trips via the request, client, and
// policies. The policies are composed around requests and will handle responses in reverse order.
func NewRequest(request *http.Request, client *http.Client, policies ...failsafe.Policy[*http.Response]) *Request {
	return NewRequestWithExecutor(request, client, failsafe.NewExecutor(policies...))
}

// NewRequestWithExecutor creates and returns a new Request that will perform failsafe round trips via the request,
// client, and executor.
func NewRequestWithExecutor(request *http.Request, client *http.Client, executor failsafe.Executor[*http.Response]) *Request {
	return &Request{
		executor: executor,
		request:  request,
		client:   client,
	}
}

func (r *Request) Do() (*http.Response, error) {
	return r.executor.GetWithExecution(func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
		ctx, cancel := util.MergeContexts(r.request.Context(), exec.Context())
		defer cancel()
		return r.client.Do(r.request.WithContext(ctx))
	})
}
