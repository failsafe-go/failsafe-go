package adaptivelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestComputeRejectionRate(t *testing.T) {
	tests := []struct {
		name               string
		rtt                time.Duration
		rejectionThreshold time.Duration
		maxExecutionTime   time.Duration
		expectedRate       float64
	}{
		{
			name:               "RTT below rejection threshold",
			rtt:                50 * time.Millisecond,
			rejectionThreshold: 100 * time.Millisecond,
			maxExecutionTime:   500 * time.Millisecond,
			expectedRate:       0.0,
		},
		{
			name:               "RTT equals rejection threshold",
			rtt:                100 * time.Millisecond,
			rejectionThreshold: 100 * time.Millisecond,
			maxExecutionTime:   500 * time.Millisecond,
			expectedRate:       0.0,
		},
		{
			name:               "RTT between threshold and max",
			rtt:                200 * time.Millisecond,
			rejectionThreshold: 100 * time.Millisecond,
			maxExecutionTime:   500 * time.Millisecond,
			expectedRate:       0.25,
		},
		{
			name:               "RTT equals max execution time",
			rtt:                500 * time.Millisecond,
			rejectionThreshold: 100 * time.Millisecond,
			maxExecutionTime:   500 * time.Millisecond,
			expectedRate:       1.0,
		},
		{
			name:               "RTT exceeds max execution time",
			rtt:                600 * time.Millisecond,
			rejectionThreshold: 100 * time.Millisecond,
			maxExecutionTime:   500 * time.Millisecond,
			expectedRate:       1.0,
		},
		{
			name:               "Small difference between threshold and max",
			rtt:                110 * time.Millisecond,
			rejectionThreshold: 100 * time.Millisecond,
			maxExecutionTime:   120 * time.Millisecond,
			expectedRate:       0.5,
		},
		{
			name:               "Threshold greater than max execution time",
			rtt:                250 * time.Millisecond,
			rejectionThreshold: 200 * time.Millisecond,
			maxExecutionTime:   100 * time.Millisecond,
			expectedRate:       1.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rate := computeRejectionRate(tc.rtt, tc.rejectionThreshold, tc.maxExecutionTime)
			assert.InDelta(t, tc.expectedRate, rate, 0.0001)
		})
	}
}
