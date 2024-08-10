package issues

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/failsafehttp"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

// See https://github.com/failsafe-go/failsafe-go/issues/65
func TestIssue65(t *testing.T) {
	test := func(t *testing.T, policies ...failsafe.Policy[*http.Response]) {
		t.Helper()
		server := testutil.MockDelayedResponseWithEarlyFlush(200, "foo", 100*time.Millisecond)
		defer server.Close()
		client := &http.Client{
			Transport: failsafehttp.NewRoundTripper(http.DefaultTransport, policies...),
		}

		resp, err := client.Get(server.URL)
		assert.NoError(t, err)
		defer resp.Body.Close()
		_, err = io.ReadAll(resp.Body)
		assert.NoError(t, err)
	}

	t.Run("without retry policy", func(t *testing.T) {
		test(t,
			policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[*http.Response](time.Nanosecond), &policytesting.Stats{}).Build(),
		)
	})

	t.Run("with retry policy", func(t *testing.T) {
		test(t,
			failsafehttp.NewRetryPolicyBuilder().Build(),
			policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[*http.Response](time.Nanosecond), &policytesting.Stats{}).Build(),
		)
	})
}
