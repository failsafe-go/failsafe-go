package adaptivelimiter

//
// import (
// 	"math"
// 	"testing"
// )
//
// func TestStabilityCalculations(t *testing.T) {
// 	tests := []struct {
// 		name    string
// 		values  []float64
// 		epsilon float64 // For floating point comparison
// 	}{
// 		{
// 			name:    "Basic sequence",
// 			values:  []float64{1.0, 2.0, 3.0, 4.0, 5.0},
// 			epsilon: 1e-10,
// 		},
// 		{
// 			name:    "All same values",
// 			values:  []float64{1.0, 1.0, 1.0, 1.0, 1.0},
// 			epsilon: 1e-10,
// 		},
// 		{
// 			name:    "Some zeros",
// 			values:  []float64{0.0, 1.0, 0.0, 2.0, 3.0},
// 			epsilon: 1e-10,
// 		},
// 		{
// 			name:    "Single value",
// 			values:  []float64{1.0},
// 			epsilon: 1e-10,
// 		},
// 		{
// 			name:    "Empty slice",
// 			values:  []float64{},
// 			epsilon: 1e-10,
// 		},
// 		{
// 			name:    "Large values",
// 			values:  []float64{1000.0, 2000.0, 3000.0, 4000.0},
// 			epsilon: 1e-10,
// 		},
// 		{
// 			name:    "Small values",
// 			values:  []float64{0.0001, 0.0002, 0.0003, 0.0004},
// 			epsilon: 1e-10,
// 		},
// 		{
// 			name:    "All zeros",
// 			values:  []float64{0.0, 0.0, 0.0},
// 			epsilon: 1e-10,
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// Original implementation
// 			originalResult := calculateStability(tt.values)
//
// 			// Sliding shortRTT implementation
// 			sliding := newVariationWindow(len(tt.values))
// 			var slidingResult float64
// 			if len(tt.values) == 0 {
// 				slidingResult = 1.0
// 			} else {
// 				for _, v := range tt.values {
// 					slidingResult = sliding.add(v)
// 				}
// 			}
//
// 			if math.Abs(originalResult-slidingResult) > tt.epsilon {
// 				t.Errorf("%s: Results differ: original=%v, sliding=%v, diff=%v",
// 					tt.name, originalResult, slidingResult, math.Abs(originalResult-slidingResult))
// 			}
// 		})
// 	}
// }
//
// // Additional edge case tests
// func TestEdgeCases(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		sequence func() (float64, float64)
// 		epsilon  float64
// 	}{
// 		{
// 			name: "Zero mean test",
// 			sequence: func() (float64, float64) {
// 				// Original implementation
// 				orig := calculateStability([]float64{0.0, 0.0})
//
// 				// Sliding implementation
// 				sliding := newVariationWindow(2)
// 				var result float64
// 				result = sliding.add(0.0)
// 				result = sliding.add(0.0)
// 				return orig, result
// 			},
// 			epsilon: 1e-10,
// 		},
// 		{
// 			name: "Sliding shortRTT overflow test",
// 			sequence: func() (float64, float64) {
// 				values := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
//
// 				// Original implementation with last 3 values
// 				orig := calculateStability([]float64{3.0, 4.0, 5.0})
//
// 				// Sliding implementation with shortRTT size 3
// 				sliding := newVariationWindow(3)
// 				var result float64
// 				for _, v := range values {
// 					result = sliding.add(v)
// 				}
// 				return orig, result
// 			},
// 			epsilon: 1e-10,
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			orig, sliding := tt.sequence()
// 			if math.Abs(orig-sliding) > tt.epsilon {
// 				t.Errorf("Results differ: original=%v, sliding=%v, diff=%v",
// 					orig, sliding, math.Abs(orig-sliding))
// 			}
// 		})
// 	}
// }
