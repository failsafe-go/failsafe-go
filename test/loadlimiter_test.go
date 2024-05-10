package test

import (
	"testing"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/loadlimiter"
)

func TestFailureSignal(t *testing.T) {
	ll := loadlimiter.With[any](loadlimiter.NewFailureSignal(), loadlimiter.NewShedAllStrategy())

	testutil.Test[any](t).
		With(ll).
		Get(func(execution failsafe.Execution[any]) (any, error) {
			return nil, nil
		})
}
