package bulkhead

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

var _ Bulkhead[any] = &bulkhead[any]{}

func TestAcquirePermit(t *testing.T) {
	bulkhead := With[any](2)

	go func() {
		time.Sleep(200 * time.Millisecond)
		bulkhead.ReleasePermit()
	}()
	elapsed := testutil.Timed(func() {
		assert.Nil(t, bulkhead.AcquirePermit(nil)) // waits 0
		assert.Nil(t, bulkhead.AcquirePermit(nil)) // waits 100
		assert.Nil(t, bulkhead.AcquirePermit(nil)) // waits 200
	})
	assert.True(t, elapsed.Milliseconds() >= 200 && elapsed.Milliseconds() <= 400)
}

func TestAcquirePermitWithMaxWaitTime(t *testing.T) {
	bulkhead := With[any](1)

	assert.Nil(t, bulkhead.AcquirePermitWithMaxWait(nil, 100*time.Millisecond)) // waits 0
	err := bulkhead.AcquirePermitWithMaxWait(nil, 100*time.Millisecond)         // waits 100
	assert.ErrorIs(t, ErrBulkheadFull, err)
}

func TestTryAcquirePermitAndReleasePermit(t *testing.T) {
	bulkhead := With[any](2)

	assert.True(t, bulkhead.TryAcquirePermit())
	assert.True(t, bulkhead.TryAcquirePermit())
	assert.False(t, bulkhead.TryAcquirePermit())

	bulkhead.ReleasePermit()
	assert.True(t, bulkhead.TryAcquirePermit())
	assert.False(t, bulkhead.TryAcquirePermit())

	bulkhead.ReleasePermit()
	bulkhead.ReleasePermit()
	assert.True(t, bulkhead.TryAcquirePermit())
	assert.True(t, bulkhead.TryAcquirePermit())
	assert.False(t, bulkhead.TryAcquirePermit())
}
