package failsafehttp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/priority"
)

const (
	priorityHeaderKey = "X-Failsafe-Priority"
	levelHeaderKey    = "X-Failsafe-Level"
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

type levelRoundTripper struct {
	next http.RoundTripper
}

// NewRoundTripperWithLevel propagates adaptivelimiter priority and level information from a client context to
// a server via HTTP headers. If a level is present it's propagated, else a priority is propagated if present.
func NewRoundTripperWithLevel(innerRoundTripper http.RoundTripper) http.RoundTripper {
	if innerRoundTripper == nil {
		innerRoundTripper = http.DefaultTransport
	}
	return &levelRoundTripper{next: innerRoundTripper}
}

func (p *levelRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	if untypedLevel := ctx.Value(priority.LevelKey); untypedLevel != nil {
		if level, ok := untypedLevel.(int); ok {
			req = req.Clone(ctx)
			req.Header.Set(levelHeaderKey, strconv.Itoa(level))
		}
	} else if untypedPriority := ctx.Value(priority.PriorityKey); untypedPriority != nil {
		if priority, ok := untypedPriority.(priority.Priority); ok {
			req = req.Clone(ctx)
			req.Header.Set(priorityHeaderKey, strconv.Itoa(int(priority)))
		}
	}

	return p.next.RoundTrip(req)
}

func (r *roundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return doRequest(request, r.executor, r.next.RoundTrip)
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
	return doRequest(r.request, r.executor, r.client.Do)
}

func doRequest(request *http.Request, executor failsafe.Executor[*http.Response], reqFn func(r *http.Request) (*http.Response, error)) (*http.Response, error) {
	bodyFunc, err := bodyReader(request.Body)
	if err != nil {
		return nil, err
	}

	// Merge the request context with the Executor so it's available for policies
	ctx, cancel := util.MergeContexts(request.Context(), executor.Context())
	defer cancel(nil)
	if ctx != executor.Context() {
		executor = executor.WithContext(ctx)
	}

	return executor.GetWithExecution(func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
		// Merge the latest execution context into the request for each attempt
		ctxInner, cancelInner := util.MergeContexts(request.Context(), exec.Context())
		defer cancelInner(nil)
		req := request
		if ctxInner != request.Context() {
			req = request.WithContext(ctxInner)
		}

		// Get new body for each attempt
		if bodyFunc != nil {
			if body, err := bodyFunc(); err != nil {
				return nil, err
			} else {
				if c, ok := body.(io.ReadCloser); ok {
					req.Body = c
				} else {
					req.Body = io.NopCloser(body)
				}
			}
		}

		return reqFn(req)
	})
}

// bodyReader returns a function that can repeatedly read the untypedBody of an http.Request.
func bodyReader(untypedBody any) (func() (io.Reader, error), error) {
	switch body := untypedBody.(type) {
	case nil:
		return nil, nil

	case *bytes.Buffer:
		return func() (io.Reader, error) {
			return bytes.NewReader(body.Bytes()), nil
		}, nil

	// Match bytes.Reader first to avoid seeking via ReadSeeker match
	case *bytes.Reader:
		buf, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		return func() (io.Reader, error) {
			return bytes.NewReader(buf), nil
		}, nil

	case io.ReadSeeker:
		return func() (io.Reader, error) {
			_, err := body.Seek(0, 0)
			return io.NopCloser(body), err
		}, nil

	case io.Reader:
		buf, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		return func() (io.Reader, error) {
			if len(buf) == 0 {
				return http.NoBody, nil
			}
			return bytes.NewReader(buf), nil
		}, nil

	default:
		return nil, fmt.Errorf("unsupported body type %T", untypedBody)
	}
}
