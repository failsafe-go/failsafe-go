package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestGetAsync(t *testing.T) {
	rp := retrypolicy.WithDefaults[bool]()
	result := failsafe.GetAsync(func() (bool, error) {
		time.Sleep(100 * time.Millisecond)
		return true, nil
	}, rp)

	assert.False(t, result.IsDone())
	<-result.Done()
	assert.True(t, result.IsDone())
	assert.True(t, result.Result())
	assert.Nil(t, result.Error())
}
