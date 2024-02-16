package examples

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
)

// This test sends a request with a 1 second delay before sending up to 2 hedges.
// The server will delay 5 seconds before responding to any of the requests.
func TestHttpHedge(t *testing.T) {
	// Setup a test http server
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		fmt.Fprintf(w, "pong")
	})
	go func() {
		http.ListenAndServe(":8080", nil)
	}()

	// Create a hedge policy
	hedgePolicy := hedgepolicy.BuilderWithDelay[any](time.Second).WithMaxHedges(2).Build()

	// Send a request with hedging
	failsafe.Run(func() error {
		fmt.Println("Sending ping")
		resp, err := http.Get("http://localhost:8080/ping")
		if err != nil {
			return err
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		fmt.Println("Received", string(body))
		return nil
	}, hedgePolicy)
}
