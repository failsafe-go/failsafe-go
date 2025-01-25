package adaptivelimiter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeError(t *testing.T) {
	testCases := []struct {
		name          string
		in            int
		out           int
		freeInflight  int
		queueSize     int
		maxQueueSize  int
		expectedError float64
	}{
		{
			name:          "No excess load",
			in:            5,
			out:           5,
			freeInflight:  10,
			queueSize:     0,
			maxQueueSize:  10,
			expectedError: -1.0,
		},
		{
			name:          "Positive excess load",
			in:            15,
			out:           5,
			freeInflight:  5,
			queueSize:     5,
			maxQueueSize:  10,
			expectedError: 0.0,
		},
		{
			name:          "Negative excess load",
			in:            3,
			out:           7,
			freeInflight:  15,
			queueSize:     2,
			maxQueueSize:  20,
			expectedError: -1.057,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := computeError(tc.in, tc.out, tc.freeInflight, tc.queueSize, tc.maxQueueSize)
			assert.InDelta(t, tc.expectedError, result, 1e-3)
		})
	}
}
