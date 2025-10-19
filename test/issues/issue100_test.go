package issues

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestAnyComposition(t *testing.T) {
	cb := circuitbreaker.NewWithDefaults[any]()
	result1, err1 := failsafe.WithAny[string](cb).Get(DoTheThing1)
	assert.Equal(t, "test", result1)
	assert.Nil(t, err1)

	rp := retrypolicy.NewWithDefaults[bool]()
	result2, err2 := failsafe.With(rp).ComposeAny(cb).Get(DoTheThing2)
	assert.True(t, result2)
	assert.Nil(t, err2)
}

func DoTheThing1() (string, error) {
	return "test", nil
}

func DoTheThing2() (bool, error) {
	return true, nil
}
