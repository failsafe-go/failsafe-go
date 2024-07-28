package retrypolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/internal/util"
)

var _ RetryPolicy[any] = &retryPolicy[any]{}

func TestRetriesExceededError(t *testing.T) {
	t.Run("with Errors.Is", func(t *testing.T) {
		e := errors.New("test")
		e1 := ExceededError{
			LastResult: false,
			LastError:  e,
		}
		e2 := ExceededError{
			LastResult: false,
			LastError:  e,
		}
		assert.ErrorIs(t, e1, e2)
	})

	t.Run("with util.ErrorTypesMatch", func(t *testing.T) {
		assert.True(t, util.ErrorTypesMatch(ExceededError{}, ErrExceeded))
		assert.True(t, util.ErrorTypesMatch(testutil.CompositeError{ExceededError{}}, ErrExceeded))
	})
}

func TestErrExceeded(t *testing.T) {
	t.Run("with Errors.Is", func(t *testing.T) {
		assert.ErrorIs(t, ExceededError{}, ErrExceeded)
		assert.ErrorIs(t, testutil.CompositeError{ExceededError{}}, ErrExceeded)
	})

	t.Run("with util.ErrorTypesMatch", func(t *testing.T) {
		assert.True(t, util.ErrorTypesMatch(ExceededError{}, ErrExceeded))
		assert.True(t, util.ErrorTypesMatch(testutil.CompositeError{ExceededError{}}, ErrExceeded))
	})
}
