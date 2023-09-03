package retrypolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"failsafe/internal/testutil"
)

var _ RetryPolicy[any] = &retryPolicy[any]{}

func TestIsAbortableNil(t *testing.T) {
	rp := OfDefaults[any]().(*retryPolicy[any])

	assert.False(t, rp.config.isAbortable(nil, nil))
}

func TestIsAbortableCompletionPredicate(t *testing.T) {
	rp := Builder[string]().AbortAllIf(func(s string, err error) bool {
		return s == "test" || errors.Is(err, testutil.InvalidArgumentError{})
	}).Build().(*retryPolicy[string])

	assert.True(t, rp.config.isAbortable("test", nil))
	assert.False(t, rp.config.isAbortable("success", nil))
	assert.True(t, rp.config.isAbortable("", testutil.InvalidArgumentError{}))
	assert.False(t, rp.config.isAbortable("", testutil.InvalidStateError{}))
}

func TestIsAbortableFailurePredicate(t *testing.T) {
	rp := Builder[string]().AbortIf(func(err error) bool {
		return errors.Is(err, testutil.InvalidArgumentError{})
	}).Build().(*retryPolicy[string])

	assert.True(t, rp.config.isAbortable("", testutil.InvalidArgumentError{}))
	assert.False(t, rp.config.isAbortable("", testutil.ConnectionError{}))
}

func TestIsAbortablePredicate(t *testing.T) {
	rp := Builder[int]().AbortResultIf(func(result int) bool {
		return result > 100
	}).Build().(*retryPolicy[int])

	assert.True(t, rp.config.isAbortable(110, nil))
	assert.False(t, rp.config.isAbortable(50, nil))
	assert.False(t, rp.config.isAbortable(50, testutil.ConnectionError{}))
}

func TestIsAbortableFailure(t *testing.T) {
	rp := Builder[any]().Abort(testutil.InvalidArgumentError{}).Build().(*retryPolicy[any])

	assert.True(t, rp.config.isAbortable(nil, testutil.InvalidArgumentError{}))
	assert.True(t, rp.config.isAbortable(nil, testutil.InvalidStateError{
		Cause: testutil.InvalidArgumentError{},
	}))
	assert.False(t, rp.config.isAbortable(nil, testutil.ConnectionError{}))
}

func TestIsAbortableResult(t *testing.T) {
	rp := Builder[any]().AbortResult(10).Build().(*retryPolicy[any])

	assert.True(t, rp.config.isAbortable(10, nil))
	assert.False(t, rp.config.isAbortable(5, nil))
	assert.False(t, rp.config.isAbortable(5, testutil.InvalidStateError{}))
}
