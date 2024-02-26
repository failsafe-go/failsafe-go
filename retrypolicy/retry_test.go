package retrypolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ RetryPolicy[any] = &retryPolicy[any]{}

func TestRetriesExceededErrorComparison(t *testing.T) {
	e := errors.New("test")
	e1 := &ExceededError{
		lastResult: false,
		lastError:  e,
	}
	e2 := &ExceededError{
		lastResult: false,
		lastError:  e,
	}
	assert.ErrorIs(t, e1, e2)
}
