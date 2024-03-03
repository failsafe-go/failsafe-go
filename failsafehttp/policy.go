package failsafehttp

import (
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

// RetryPolicyBuilder returns a retrypolicy.RetryPolicyBuilder that will retry non-terminal HTTP errors and responses up
// to 2 times, by default. If a Retry-After header is present in the response, it will be used as a delay between
// retries. Additional handling and delay configuration can be added to the resulting builder.
func RetryPolicyBuilder() retrypolicy.RetryPolicyBuilder[*http.Response] {
	retryHandleFunc := func(resp *http.Response, err error) bool {
		// Handle errors
		if err != nil {
			if v, ok := err.(*url.Error); ok {
				// Do not retry when certain error messages are observed
				if unsupportedScheme.MatchString(v.Error()) ||
					certNotTrusted.MatchString(v.Error()) ||
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

	return retrypolicy.Builder[*http.Response]().
		HandleIf(retryHandleFunc).
		WithDelayFunc(DelayFunc())
}

// DelayFunc returns a failsafe.DelayFunc that delays according to an http.Response Retry-After header. This can be used
// as a delay in a RetryPolicy or a CircuitBreaker.
func DelayFunc() failsafe.DelayFunc[*http.Response] {
	return func(exec failsafe.ExecutionAttempt[*http.Response]) time.Duration {
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
}
