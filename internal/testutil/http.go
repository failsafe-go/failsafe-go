package testutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"time"
)

func MockResponse(statusCode int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(statusCode)
		fmt.Fprintf(w, body)
	}))
}

func MockFlakyServer(failTimes int, responseCode int, retryAfterDelay time.Duration, finalResponse string) (server *httptest.Server, resetFailures func()) {
	failures := atomic.Int32{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if failures.Add(1) <= int32(failTimes) {
				if retryAfterDelay > 0 {
					w.Header().Add("Retry-After", strconv.Itoa(int(retryAfterDelay.Seconds())))
				}
				fmt.Println("Replying with", responseCode)
				w.WriteHeader(responseCode)
			} else {
				fmt.Fprintf(w, finalResponse)
			}
		})), func() {
			failures.Swap(0)
		}
}

func MockDelayedResponse(statusCode int, body string, delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			w.WriteHeader(statusCode)
			fmt.Fprintf(w, body)
		case <-request.Context().Done():
			timer.Stop()
		}
	}))
}

func MockDelayedResponseWithEarlyFlush(statusCode int, body string, delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(statusCode)
		w.(http.Flusher).Flush() // Ensure data and error is sent to the client
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			fmt.Fprintf(w, body)
		case <-request.Context().Done():
			timer.Stop()
		}
	}))
}
