package failsafehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

type tester struct {
	tester   *testutil.Tester[*http.Response]
	handler  http.HandlerFunc
	server   *httptest.Server
	url      string
	reqCtxFn func() context.Context
}

func test(t *testing.T, server *httptest.Server) *tester {
	return &tester{
		tester: testutil.Test[*http.Response](t),
		server: server,
	}
}

func (t *tester) Url(url string) *tester {
	t.url = url
	return t
}

func (t *tester) Setup(fn func()) *tester {
	t.tester.Setup(fn)
	return t
}

func (t *tester) RequestContext(fn func() context.Context) *tester {
	t.reqCtxFn = fn
	return t
}

func (t *tester) Reset(stats ...testutil.Resetable) *tester {
	t.tester.Reset(stats...)
	return t
}

func (t *tester) With(policies ...failsafe.Policy[*http.Response]) *tester {
	t.tester.With(policies...)
	return t
}

func (t *tester) WithExecutor(executor failsafe.Executor[*http.Response]) *tester {
	t.tester.WithExecutor(executor)
	return t
}

func (t *tester) AssertSuccess(expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult string, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, expectedStatus, expectedResult, nil, true, false, then...)
}

func (t *tester) AssertSuccessError(expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, 0, "", expectedError, true, false, then...)
}

func (t *tester) AssertFailure(expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, 0, "", expectedError, false, true, then...)
}

func (t *tester) AssertFailureResult(expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult string, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, expectedStatus, expectedResult, nil, false, true, then...)
}

// Asserts an error with no execution having taken place.
func (t *tester) AssertError(expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, 0, "", expectedError, false, false, then...)
}

func (t *tester) assertResult(expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult string, expectedError error, expectedSuccess bool, expectedFailure bool, then ...func()) {
	t.tester.T.Helper()
	if t.tester.Executor == nil {
		t.tester.Executor = failsafe.NewExecutor[*http.Response](t.tester.Policies...)
	}

	executorFn, assertResult := testutil.PrepareTest(t.tester.T, t.tester.SetupFn, nil, t.tester.Executor)
	assertHttpResult := func(resp *http.Response, err error) {
		// Read body
		var body string
		if resp != nil {
			t.tester.T.Cleanup(func() {
				resp.Body.Close()
			})
			bodyBytes, err := io.ReadAll(resp.Body)
			if err == nil {
				body = strings.TrimSpace(string(bodyBytes))
			}
		}

		// Assert result
		if expectedResult != "" {
			assert.Equal(t.tester.T, expectedResult, body)
		}

		// Unwrap and assert URL errors
		expectedErrCopy := expectedError
		var urlErr1 *url.Error
		var urlErr2 *url.Error
		if errors.As(err, &urlErr1) && errors.As(expectedError, &urlErr2) {
			assert.Equal(t.tester.T, urlErr1.Err.Error(), urlErr2.Err.Error(), "expected error did not match")
			// Clear error vars so that assertResult doesn't assert them again
			expectedErrCopy = nil
			err = nil
		}

		// Assert status
		if resp != nil && expectedStatus > 0 {
			assert.Equal(t.tester.T, expectedStatus, resp.StatusCode)
		}

		// Assert remaining error and events
		assertResult(expectedAttempts, expectedExecutions, nil, nil, expectedErrCopy, err, expectedSuccess, expectedFailure, false, then...)
	}
	ctxFn := func() context.Context {
		if t.reqCtxFn != nil {
			return t.reqCtxFn()
		}
		if t.tester.ContextFn != nil {
			return t.tester.ContextFn()
		}
		return context.Background()
	}
	if t.url == "" && t.server != nil {
		t.url = t.server.URL
	}

	if t.server == nil {
		// Test server
		t.server = httptest.NewServer(NewHandlerWithExecutor(t.handler, executorFn()))
		client := http.Client{}
		req, _ := http.NewRequestWithContext(ctxFn(), http.MethodGet, t.server.URL, nil)
		assertHttpResult(client.Do(req))
	} else {
		// Test client with roundtripper
		fmt.Println("Testing RoundTripper")
		assertHttpResult(testRoundTripper(ctxFn(), t.url, executorFn()))

		// Test client with failsafehttp.Request
		fmt.Println("\nTesting failsafehttp.Request")
		assertHttpResult(testRequest(ctxFn(), t.url, executorFn()))
	}

	if t.server != nil {
		t.server.Close()
	}
}

func testRoundTripper(ctx context.Context, path string, executor failsafe.Executor[*http.Response]) (*http.Response, error) {
	client := http.Client{Transport: NewRoundTripperWithExecutor(nil, executor)}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	return client.Do(req)
}

func testRequest(ctx context.Context, path string, executor failsafe.Executor[*http.Response]) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	return NewRequestWithExecutor(req, http.DefaultClient, executor).Do()
}
