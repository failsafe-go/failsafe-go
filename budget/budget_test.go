package budget

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ Budget[any] = &budget[any]{}

func TestTryAcquirePermitAndReleasePermit(t *testing.T) {
	b := NewBuilder[any]().
		ForRetries().
		WithMaxRate(.5).
		WithMinConcurrency(1).
		Build().(*budget[any])
	b.executions.Add(1)

	assert.True(t, b.TryAcquireRetryPermit())
	assert.True(t, b.TryAcquireRetryPermit())
	assert.False(t, b.TryAcquireRetryPermit())

	b.ReleaseRetryPermit()
	assert.True(t, b.TryAcquireRetryPermit())
	assert.False(t, b.TryAcquireRetryPermit())

	b.ReleaseRetryPermit()
	b.ReleaseRetryPermit()
	assert.True(t, b.TryAcquireRetryPermit())
	assert.True(t, b.TryAcquireRetryPermit())
	assert.False(t, b.TryAcquireRetryPermit())
}
