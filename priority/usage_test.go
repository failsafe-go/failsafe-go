package priority

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUsageTracker_RecordUsage(t *testing.T) {
	t.Run("with a single user", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(10, time.Minute)
		userID := "user1"
		duration := 100 * time.Millisecond

		// When
		tracker.RecordUsage(context.Background(), userID, duration)

		// Then
		usage := tracker.GetUsage(userID)
		assert.Greater(t, usage, 0.0)
		assert.Equal(t, float64(duration.Nanoseconds()), usage)
	})

	t.Run("with multiple users", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(10, time.Minute)
		ctx := context.Background()

		// When
		tracker.RecordUsage(ctx, "user1", 100*time.Millisecond)
		tracker.RecordUsage(ctx, "user1", 50*time.Millisecond)
		tracker.RecordUsage(ctx, "user2", 200*time.Millisecond)

		// Then
		usage1 := tracker.GetUsage("user1")
		usage2 := tracker.GetUsage("user2")

		assert.Equal(t, float64(150*time.Millisecond.Nanoseconds()), usage1)
		assert.Equal(t, float64(200*time.Millisecond.Nanoseconds()), usage2)
	})
}

func TestUsageTracker_GetLevel(t *testing.T) {
	t.Run("with an unknown user", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(10, time.Minute)

		// When
		level := tracker.GetLevel("unknown", High)

		// Then
		assert.Equal(t, High.MaxLevel(), level)
	})

	t.Run("with an uncalibrated user", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(10, time.Minute)

		// When
		tracker.RecordUsage(context.Background(), "user1", 100*time.Millisecond)
		level := tracker.GetLevel("user1", High)

		// Then
		assert.Equal(t, High.MaxLevel(), level)
	})

	t.Run("with different priorities", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(10, time.Minute)
		tracker.RecordUsage(context.Background(), "user1", 100*time.Millisecond)
		tracker.Calibrate()

		// When
		veryLowLevel := tracker.GetLevel("user1", VeryLow)
		lowLevel := tracker.GetLevel("user1", Low)
		mediumLevel := tracker.GetLevel("user1", Medium)
		highLevel := tracker.GetLevel("user1", High)
		veryHighLevel := tracker.GetLevel("user1", VeryHigh)

		// Then
		assert.True(t, veryLowLevel >= 0 && veryLowLevel <= 99)
		assert.True(t, lowLevel >= 100 && lowLevel <= 199)
		assert.True(t, mediumLevel >= 200 && mediumLevel <= 299)
		assert.True(t, highLevel >= 300 && highLevel <= 399)
		assert.True(t, veryHighLevel >= 400 && veryHighLevel <= 499)
	})

	t.Run("with calibrated users with different usage", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(10, time.Minute)
		ctx := context.Background()
		tracker.RecordUsage(ctx, "user1", 50*time.Millisecond)
		tracker.RecordUsage(ctx, "user2", 100*time.Millisecond)
		tracker.RecordUsage(ctx, "user3", 200*time.Millisecond)

		// When
		tracker.Calibrate()
		level1 := tracker.GetLevel("user1", Medium)
		level2 := tracker.GetLevel("user2", Medium)
		level3 := tracker.GetLevel("user3", Medium)

		// Then
		assert.Greater(t, level1, level3)
		assert.GreaterOrEqual(t, level2, level3)
		assert.GreaterOrEqual(t, level1, level2)
	})
}

func TestUsageTracker_LRU(t *testing.T) {
	t.Run("when a user is evicted", func(t *testing.T) {
		// Given
		maxUsers := 2
		tracker := NewUsageTracker(maxUsers, time.Minute)
		ctx := context.Background()
		tracker.RecordUsage(ctx, "user1", 100*time.Millisecond)
		tracker.RecordUsage(ctx, "user2", 200*time.Millisecond)

		// when
		tracker.RecordUsage(ctx, "user3", 300*time.Millisecond)

		// Then
		assert.Equal(t, 0.0, tracker.GetUsage("user1"))
		assert.Greater(t, tracker.GetUsage("user2"), 0.0)
		assert.Greater(t, tracker.GetUsage("user3"), 0.0)
	})

	t.Run("when the most recent user changes", func(t *testing.T) {
		// Given
		maxUsers := 2
		tracker := NewUsageTracker(maxUsers, time.Minute)
		ctx := context.Background()
		tracker.RecordUsage(ctx, "user1", 100*time.Millisecond)
		tracker.RecordUsage(ctx, "user2", 200*time.Millisecond)

		// When
		tracker.RecordUsage(ctx, "user1", 50*time.Millisecond)
		tracker.RecordUsage(ctx, "user3", 300*time.Millisecond)

		// Then
		assert.Greater(t, tracker.GetUsage("user1"), 0.0)
		assert.Equal(t, 0.0, tracker.GetUsage("user2"))
		assert.Greater(t, tracker.GetUsage("user3"), 0.0)
	})
}

func TestUsageTracker_ComputeQuantile(t *testing.T) {
	// Given
	tracker := NewUsageTracker(10, time.Minute).(*usageTracker)

	tests := []struct {
		name         string
		usage        float64
		sortedUsages []float64
		expected     float64
	}{
		{
			name:         "with no usages",
			usage:        100,
			sortedUsages: []float64{},
			expected:     0,
		},
		{
			name:         "with one usage - match",
			usage:        100,
			sortedUsages: []float64{100},
			expected:     0,
		},
		{
			name:         "with one usage - lower",
			usage:        50,
			sortedUsages: []float64{100},
			expected:     0,
		},
		{
			name:         "with one usage - higher",
			usage:        150,
			sortedUsages: []float64{100},
			expected:     1,
		},
		{
			name:         "with multiple usages - lowest",
			usage:        10,
			sortedUsages: []float64{10, 50, 100},
			expected:     0,
		},
		{
			name:         "with multiple usages - middle",
			usage:        50,
			sortedUsages: []float64{10, 50, 100},
			expected:     0.33,
		},
		{
			name:         "with multiple usages - highest",
			usage:        100,
			sortedUsages: []float64{10, 50, 100},
			expected:     0.67,
		},
		{
			name:         "with multiple usages - higher",
			usage:        200,
			sortedUsages: []float64{10, 50, 100},
			expected:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			quantile := tracker.computeQuantile(tt.usage, tt.sortedUsages)

			// Then
			assert.Equal(t, tt.expected, quantile)
		})
	}
}

func TestContextWithUserID(t *testing.T) {
	userID := "test-user"
	newCtx := ContextWithUserID(context.Background(), userID)
	assert.Equal(t, userID, newCtx.Value(UserKey))
}

func TestUserFromContext(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name:     "with user ID",
			ctx:      ContextWithUserID(context.Background(), "test-user"),
			expected: "test-user",
		},
		{
			name:     "without user ID",
			ctx:      context.Background(),
			expected: "",
		},
		{
			name:     "with wrong type value",
			ctx:      context.WithValue(context.Background(), UserKey, 123),
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			userID := UserFromContext(tc.ctx)
			assert.Equal(t, tc.expected, userID)
		})
	}
}
