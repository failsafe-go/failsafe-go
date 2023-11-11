package retrypolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

var _ RetryPolicy[any] = &retryPolicy[any]{}

func TestIsAbortableNil(t *testing.T) {
	rp := WithDefaults[any]().(*retryPolicy[any])

	assert.False(t, rp.config.isAbortable(nil, nil))
}

func TestIsAbortableForError(t *testing.T) {
	rp := Builder[any]().AbortOnErrors(testutil.ErrInvalidArgument).Build().(*retryPolicy[any])

	assert.True(t, rp.config.isAbortable(nil, testutil.ErrInvalidArgument))
	assert.True(t, rp.config.isAbortable(nil, testutil.NewCompositeError(testutil.ErrInvalidArgument)))
	assert.False(t, rp.config.isAbortable(nil, testutil.ErrConnecting))
}

func TestIsAbortableForResult(t *testing.T) {
	rp := Builder[any]().AbortOnResult(10).Build().(*retryPolicy[any])

	assert.True(t, rp.config.isAbortable(10, nil))
	assert.False(t, rp.config.isAbortable(5, nil))
	assert.False(t, rp.config.isAbortable(5, testutil.ErrInvalidState))
}

func TestIsAbortableForPredicate(t *testing.T) {
	rp := Builder[any]().AbortIf(func(s any, err error) bool {
		return s == "test" || errors.Is(err, testutil.ErrInvalidArgument)
	}).Build().(*retryPolicy[any])

	assert.True(t, rp.config.isAbortable("test", nil))
	assert.False(t, rp.config.isAbortable(0, nil))
	assert.True(t, rp.config.isAbortable("", testutil.ErrInvalidArgument))
	assert.False(t, rp.config.isAbortable("", testutil.ErrInvalidState))
}

func TestRetriesExceededErrorComparison(t *testing.T) {
	e := errors.New("test")
	e1 := &RetriesExceededError{
		lastResult: false,
		lastError:  e,
	}
	e2 := &RetriesExceededError{
		lastResult: false,
		lastError:  e,
	}
	assert.ErrorIs(t, e1, e2)
}
