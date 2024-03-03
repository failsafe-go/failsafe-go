package examples

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/failsafehttp"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// Test test demonstrates how to use a RetryPolicy with HTTP using two different approaches:
//   - a failsafe http.RoundTripper
//   - a failsafe execution
func TestRetryPolicyWithHttp(t *testing.T) {
	// Setup a test http server that returns 400 on the first two requests
	counter := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if counter.Add(1) < 3 {
			fmt.Println("replying with 400")
			w.WriteHeader(400)
		} else {
			fmt.Fprintf(w, "pong")
		}
	}))
	defer server.Close()

	// Create a RetryPolicy that handles 400 responses
	retryPolicy := retrypolicy.Builder[*http.Response]().HandleIf(func(response *http.Response, err error) bool {
		return response.StatusCode == 400
	}).Build()

	// Demonstrates how to use a RetryPoilicy with a failsafe RoundTripper
	t.Run("with failsafe round tripper", func(t *testing.T) {
		executor := failsafe.NewExecutor[*http.Response](retryPolicy)
		roundTripper := failsafehttp.NewRoundTripper(executor, nil)
		client := &http.Client{Transport: roundTripper}

		fmt.Println("Sending ping")
		resp, err := client.Get(server.URL)
		if err != nil {
			return
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}

		fmt.Println("Received", string(body))
	})

	// Demonstrates how to use a RetryPoilicy with an HTTP client via a failsafe execution
	t.Run("with failsafe execution", func(t *testing.T) {
		counter.Store(0)
		fmt.Println("Sending ping")

		// Perform a failsafe execution
		resp, err := failsafe.GetWithExecution(func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
			req, _ := http.NewRequestWithContext(exec.Context(), http.MethodGet, server.URL, nil)
			client := &http.Client{}
			return client.Do(req)
		}, retryPolicy)

		if err != nil {
			return
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}

		fmt.Println("Received", string(body))
	})
}

// This test demonstrates how to use a HedgePolicy with HTTP using two different approaches:
//
//   - a failsafe http.RoundTripper
//   - a failsafe execution
//
// Each test will send a request and the HedgePolicy will delay for 1 second before sending up to 2 hedges. The server
// will delay 5 seconds before responding to any of the requests. After the first successul response is received by the
// client, the context for any outstanding requests will be canceled.
func TestHedgePolicyWithHttp(t *testing.T) {
	// Setup a test http server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		timer := time.After(5 * time.Second)
		select {
		// request.Context() will be done as soon as the first successful response is handled by the client
		case <-request.Context().Done():
		case <-timer:
			fmt.Fprintf(w, "pong")
		}
	}))
	defer server.Close()

	// Create a HedgePolicy that sends up to 2 hedges after a 1 second delay each
	hedgePolicy := hedgepolicy.BuilderWithDelay[*http.Response](time.Second).
		WithMaxHedges(2).
		OnHedge(func(f failsafe.ExecutionEvent[*http.Response]) {
			fmt.Println("Sending hedge request")
		}).
		Build()

	// Demonstrates how to use a HedgePolicy with a failsafe RoundTripper
	t.Run("with failsafe round tripper", func(t *testing.T) {
		// Create a client with a failsafe RoundTripper
		executor := failsafe.NewExecutor[*http.Response](hedgePolicy)
		roundTripper := failsafehttp.NewRoundTripper(executor, nil)
		client := &http.Client{Transport: roundTripper}

		fmt.Println("Sending ping")
		resp, err := client.Get(server.URL)
		if err != nil {
			return
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}

		fmt.Println("Received", string(body))
	})

	// Demonstrates how to use a HedgePolicy with an HTTP client via a failsafe execution
	t.Run("with failsafe execution", func(t *testing.T) {
		fmt.Println("Sending ping")

		// Perform a failsafe execution
		resp, err := failsafe.GetWithExecution(func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
			req, _ := http.NewRequestWithContext(exec.Context(), http.MethodGet, server.URL, nil)
			client := &http.Client{}
			return client.Do(req)
		}, hedgePolicy)

		if err != nil {
			return
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}

		fmt.Println("Received", string(body))
	})
}
