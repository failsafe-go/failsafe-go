package failsafehttp

import (
	"net/http"

	"github.com/failsafe-go/failsafe-go"
)

type roundTripper struct {
	next     http.RoundTripper
	executor failsafe.Executor[*http.Response]
}

// NewRoundTripper creates and returns a new http.RoundTripper that will perform failsafe round trips via the executor
// and innerRoundTripper. If innerRoundTripper is nil, http.DefaultTransport will be used.
func NewRoundTripper(executor failsafe.Executor[*http.Response], innerRoundTripper http.RoundTripper) http.RoundTripper {
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
		return f.next.RoundTrip(request.WithContext(exec.Context()))
	})
}

type Request struct {
	executor failsafe.Executor[*http.Response]
	request  *http.Request
	client   *http.Client
}

func NewRequest(executor failsafe.Executor[*http.Response], request *http.Request, client *http.Client) *Request {
	return &Request{
		executor: executor,
		request:  request,
		client:   client,
	}
}

func (c *Request) Do() (*http.Response, error) {
	return c.executor.GetWithExecution(func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
		return c.client.Do(c.request.WithContext(exec.Context()))
	})
}
