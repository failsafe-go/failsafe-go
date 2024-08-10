package failsafehttp

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

var (
	unsupportedScheme     = regexp.MustCompile(`unsupported protocol scheme`)
	certNotTrusted        = regexp.MustCompile(`certificate is not trusted`)
	stoppedAfterRedirects = regexp.MustCompile(`stopped after \d+ redirects\z`)
)

// NewRetryPolicyBuilder returns a retrypolicy.Builder that will retry non-terminal HTTP errors and responses up
// to 2 times, by default. If a Retry-After header is present in the response, it will be used as a delay between
// retries. Additional handling and delay configuration can be added to the resulting builder.
func NewRetryPolicyBuilder() retrypolicy.Builder[*http.Response] {
	retryHandleFunc := func(resp *http.Response, err error) bool {
		// Handle errors
		if err != nil {
			// Do not retry unsupported protocol scheme error
			// This will be a url.Error when using an http.Client, and an errorString when using a RoundTripper
			if unsupportedScheme.MatchString(err.Error()) {
				return false
			}
			if v, ok := err.(*url.Error); ok {
				// Do not retry when certain error messages are observed
				if certNotTrusted.MatchString(v.Error()) ||
					stoppedAfterRedirects.MatchString(v.Error()) {
					return false
				}
				// Do not retry on unknown authority errors
				if _, ok := v.Err.(x509.UnknownAuthorityError); ok {
					return false
				}
			}
			// Retry on all other url errors
			return true
		}

		// Handle response
		if resp != nil {
			// Retry on 429
			if resp.StatusCode == http.StatusTooManyRequests {
				return true
			}
			// Retry on most 5xx responses
			if resp.StatusCode >= 500 && resp.StatusCode != http.StatusNotImplemented {
				return true
			}
		}

		return false
	}

	return retrypolicy.NewBuilder[*http.Response]().
		HandleIf(retryHandleFunc).
		AbortOnErrors(context.Canceled).
		WithDelayFunc(DelayFunc)
}

// DelayFunc delays according to an http.Response Retry-After header. This can be used as a delay in a RetryPolicy or a CircuitBreaker.
func DelayFunc(exec failsafe.ExecutionAttempt[*http.Response]) time.Duration {
	resp := exec.LastResult()
	if resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) {
		if header, ok := resp.Header["Retry-After"]; ok {
			if seconds, err := strconv.Atoi(header[0]); err == nil {
				return time.Second * time.Duration(seconds)
			}
		}
	}

	return -1
}
