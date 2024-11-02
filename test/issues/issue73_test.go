package issues

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/failsafehttp"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// See https://github.com/failsafe-go/failsafe-go/issues/73
func TestIssue73(t *testing.T) {
	retryPolicy := failsafehttp.RetryPolicyBuilder().
		HandleIf(func(response *http.Response, err error) bool {
			return true
		}).
		OnRetry(func(e failsafe.ExecutionEvent[*http.Response]) {
			fmt.Println("retrying")
		}).Build()

	t.Run("with RoundTripper", func(t *testing.T) {
		client := &http.Client{
			Transport: failsafehttp.NewRoundTripper(nil, retryPolicy),
		}
		server, requestsWithBody := createServer()
		defer server.Close()

		req, err := http.NewRequest(http.MethodGet, server.URL, strings.NewReader("ping"))
		resp, err := client.Do(req)
		var exceeded retrypolicy.ExceededError
		if errors.As(err, &exceeded) {
			resp = exceeded.LastResult.(*http.Response)
		}
		body, _ := io.ReadAll(resp.Body)
		fmt.Println("received response", string(body))
		resp.Body.Close()
		assert.Equal(t, requestsWithBody.Load(), int32(3))
	})

	t.Run("with failsafehttp.Request", func(t *testing.T) {
		server, requestsWithBody := createServer()
		defer server.Close()

		req, err := http.NewRequest(http.MethodGet, server.URL, strings.NewReader("ping"))
		request := failsafehttp.NewRequest(req, &http.Client{}, retryPolicy)
		resp, err := request.Do()
		var exceeded retrypolicy.ExceededError
		if errors.As(err, &exceeded) {
			resp = exceeded.LastResult.(*http.Response)
		}
		body, _ := io.ReadAll(resp.Body)
		fmt.Println("received response", string(body))
		resp.Body.Close()
		assert.Equal(t, requestsWithBody.Load(), int32(3))
	})
}

func createServer() (*httptest.Server, *atomic.Int32) {
	var requestsWithBody = atomic.Int32{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		req, _ := io.ReadAll(request.Body)
		if string(req) == "ping" {
			requestsWithBody.Add(1)
		}
		fmt.Println("received request", string(req))
		fmt.Fprintf(w, "pong")
	})), &requestsWithBody
}
