package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRoundDown(t *testing.T) {
	assert.Equal(t, time.Duration(0), RoundDown(0, 20))
	assert.Equal(t, time.Duration(40), RoundDown(40, 20))
	assert.Equal(t, time.Duration(40), RoundDown(57, 20))
	assert.Equal(t, time.Duration(45), RoundDown(57, 15))
}

func TestRandomDelayInRange(t *testing.T) {
	assert.Equal(t, 10, RandomDelayInRange(10, 100, 0))
	assert.Equal(t, 32, RandomDelayInRange(10, 100, .25))
	assert.Equal(t, 55, RandomDelayInRange(10, 100, .5))
	assert.Equal(t, 77, RandomDelayInRange(10, 100, .75))
	assert.Equal(t, 100, RandomDelayInRange(10, 100, 1))

	assert.Equal(t, 162, RandomDelayInRange(50, 500, .25))
	assert.Equal(t, 16250, RandomDelayInRange(5000, 50000, .25))
}

func TestRandomDelayForFactor(t *testing.T) {
	assert.Equal(t, 150, RandomDelayFactor(100, .5, 0))
	assert.Equal(t, 125, RandomDelayFactor(100, .5, .25))
	assert.Equal(t, 100, RandomDelayFactor(100, .5, .5))
	assert.Equal(t, 75, RandomDelayFactor(100, .5, .75))
	assert.Equal(t, 50, RandomDelayFactor(100, .5, .9999))

	assert.Equal(t, 625, RandomDelayFactor(500, .5, .25))
	assert.Equal(t, 375, RandomDelayFactor(500, .5, .75))
	assert.Equal(t, 62500, RandomDelayFactor(50000, .5, .25))
}

func TestRandomDelayForDuration(t *testing.T) {
	assert.Equal(t, 150, RandomDelay(100, 50, 0))
	assert.Equal(t, 125, RandomDelay(100, 50, .25))
	assert.Equal(t, 100, RandomDelay(100, 50, .5))
	assert.Equal(t, 75, RandomDelay(100, 50, .75))
	assert.Equal(t, 50, RandomDelay(100, 50, 1))

	assert.Equal(t, 525, RandomDelay(500, 50, .25))
	assert.Equal(t, 52500, RandomDelay(50000, 5000, .25))
}
