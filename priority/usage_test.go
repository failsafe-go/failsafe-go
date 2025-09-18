package priority

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestUsageTracker_RecordUsage(t *testing.T) {
	t.Run("with a single user", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(time.Minute, 10)
		userID := "user1"
		usage := int64(100)

		// When
		tracker.RecordUsage(userID, usage)

		// Then
		actual, _ := tracker.GetUsage(userID)
		assert.Equal(t, usage, actual)
	})

	t.Run("with multiple users", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(time.Minute, 10)

		// When
		tracker.RecordUsage("user1", 100)
		tracker.RecordUsage("user1", 50)
		tracker.RecordUsage("user2", 200)

		// Then
		usage1, _ := tracker.GetUsage("user1")
		usage2, _ := tracker.GetUsage("user2")

		assert.Equal(t, int64(150), usage1)
		assert.Equal(t, int64(200), usage2)
	})
}

func TestUsageTracker_GetLevel(t *testing.T) {
	t.Run("with an unknown user", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(time.Minute, 10)

		// When
		level := tracker.GetLevel("unknown", High)

		// Then
		assert.Equal(t, High.MaxLevel(), level)
	})

	t.Run("with an uncalibrated user", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(time.Minute, 10)

		// When
		tracker.RecordUsage("user1", 100)
		level := tracker.GetLevel("user1", High)

		// Then
		assert.Equal(t, High.MaxLevel(), level)
	})

	t.Run("with different priorities", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(time.Minute, 10)
		tracker.RecordUsage("user1", 100)
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
		tracker := NewUsageTracker(time.Minute, 10)
		tracker.RecordUsage("user1", 50)
		tracker.RecordUsage("user2", 100)
		tracker.RecordUsage("user3", 200)

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
	t.Run("should limit to max users", func(t *testing.T) {
		// Given
		tracker := NewUsageTracker(time.Minute, 2)
		tracker.RecordUsage("user1", 100)
		tracker.RecordUsage("user2", 200)
		_, user1Exists := tracker.GetUsage("user1")
		_, user2Exists := tracker.GetUsage("user2")
		assert.True(t, user1Exists)
		assert.True(t, user2Exists)

		// When
		tracker.RecordUsage("user3", 300)
		_, user1Exists = tracker.GetUsage("user1")
		_, user2Exists = tracker.GetUsage("user2")
		_, user3Exists := tracker.GetUsage("user3")
		assert.False(t, user1Exists)
		assert.True(t, user2Exists)
		assert.True(t, user3Exists)
	})
}

func TestUsageTracker_Cleanup(t *testing.T) {
	createTracker := func() (*usageTracker, *testutil.TestClock) {
		tracker := NewUsageTracker(100*time.Millisecond, 10).(*usageTracker)
		clock := testutil.NewTestClock(0)
		tracker.clock = clock
		tracker.newWindowFn = func() *usageWindow {
			return newUsageWindow(30, 100*time.Millisecond, clock)
		}
		return tracker, clock
	}

	// Asserts that cleanup removes an inactive user, leaving space for a new user
	t.Run("should allow more users after cleanup", func(t *testing.T) {
		// Given
		tracker, clock := createTracker()
		tracker.RecordUsage("user1", 100)
		tracker.RecordUsage("user2", 200)
		_, user1Exists := tracker.GetUsage("user1")
		_, user2Exists := tracker.GetUsage("user2")
		assert.True(t, user1Exists)
		assert.True(t, user2Exists)

		// When
		clock.SetTime(110)
		tracker.RecordUsage("active_user", 50)
		clock.SetTime(210)
		tracker.RecordUsage("active_user2", 80)
		tracker.Calibrate()

		// Then
		activeUsage, activeOk := tracker.GetUsage("active_user")
		_, inactiveOk := tracker.GetUsage("inactive_user")
		activeUsage2, active2Ok := tracker.GetUsage("active_user2")
		assert.Equal(t, int64(0), activeUsage)
		assert.True(t, activeOk)
		assert.False(t, inactiveOk)
		assert.Equal(t, int64(80), activeUsage2)
		assert.True(t, active2Ok)
	})
}

func TestUsageTracker_ComputeQuantile(t *testing.T) {
	// Given
	tracker := NewUsageTracker(time.Minute, 10).(*usageTracker)

	tests := []struct {
		name         string
		usage        int64
		sortedUsages []int64
		expected     float64
	}{
		{
			name:         "with no usages",
			usage:        100,
			sortedUsages: []int64{},
			expected:     0,
		},
		{
			name:         "with one usage - match",
			usage:        100,
			sortedUsages: []int64{100},
			expected:     0,
		},
		{
			name:         "with one usage - lower",
			usage:        50,
			sortedUsages: []int64{100},
			expected:     0,
		},
		{
			name:         "with one usage - higher",
			usage:        150,
			sortedUsages: []int64{100},
			expected:     1,
		},
		{
			name:         "with multiple usages - lowest",
			usage:        10,
			sortedUsages: []int64{10, 50, 100},
			expected:     0,
		},
		{
			name:         "with multiple usages - middle",
			usage:        50,
			sortedUsages: []int64{10, 50, 100},
			expected:     0.33,
		},
		{
			name:         "with multiple usages - highest",
			usage:        100,
			sortedUsages: []int64{10, 50, 100},
			expected:     0.67,
		},
		{
			name:         "with multiple usages - higher",
			usage:        200,
			sortedUsages: []int64{10, 50, 100},
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
	newCtx := ContextWithUser(context.Background(), userID)
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
			ctx:      ContextWithUser(context.Background(), "test-user"),
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

func TestUsageWindow(t *testing.T) {
	clock := testutil.NewTestClock(900)

	// Given 4 buckets representing 1 second each
	window := newUsageWindow(4, 4*time.Second, clock)
	assert.Equal(t, int64(0), window.TotalUsage())
	assert.Equal(t, uint32(0), window.Samples())

	// Record into bucket 1
	recordUsage(window, []int64{100, 200, 150, 300, 250}) // currentTime = 0
	assert.Equal(t, int64(0), window.HeadTime)
	assert.Equal(t, int64(1000), window.TotalUsage())
	assert.Equal(t, uint32(5), window.Samples())

	// Record into bucket 2
	clock.SetTime(1000)
	recordUsage(window, []int64{400, 500})
	assert.Equal(t, int64(1), window.HeadTime)
	assert.Equal(t, int64(1900), window.TotalUsage()) // 1000 + 900
	assert.Equal(t, uint32(7), window.Samples())

	// Record into bucket 3
	clock.SetTime(2500)
	recordUsage(window, []int64{600, 700, 800})
	assert.Equal(t, int64(2), window.HeadTime)
	assert.Equal(t, int64(4000), window.TotalUsage()) // 1900 + 2100
	assert.Equal(t, uint32(10), window.Samples())

	// Record into bucket 4
	clock.SetTime(3100)
	recordUsage(window, []int64{50, 75})
	assert.Equal(t, int64(3), window.HeadTime)
	assert.Equal(t, int64(4125), window.TotalUsage()) // 4000 + 125
	assert.Equal(t, uint32(12), window.Samples())

	// Record into bucket 2, skipping bucket 1
	clock.SetTime(5400)
	recordUsage(window, []int64{500})
	assert.Equal(t, int64(5), window.HeadTime)

	// Assert bucket 1 was skipped and reset based on its previous start time
	bucket1 := window.Buckets[0]
	assert.Equal(t, int64(0), bucket1.totalUsage)
	assert.Equal(t, uint32(0), bucket1.samples)

	// Should have lost bucket 1's data (1000) and gained new data (500)
	assert.Equal(t, int64(2725), window.TotalUsage()) // 4125 - 1000 - 900 + 500
	assert.Equal(t, uint32(6), window.Samples())      // 12 - 5 - 2 + 1

	// Record into bucket 4, skipping bucket 3
	clock.SetTime(7300)
	recordUsage(window, []int64{300, 400})
	assert.Equal(t, int64(7), window.HeadTime)

	// Assert bucket 3 was skipped and reset
	bucket3 := window.Buckets[2]
	assert.Equal(t, int64(0), bucket3.totalUsage)
	assert.Equal(t, uint32(0), bucket3.samples)

	// Should have lost bucket 3's data (2100) and gained new data (700)
	assert.Equal(t, int64(1200), window.TotalUsage()) // 2725 - 2100 - 125 + 700
	assert.Equal(t, uint32(3), window.Samples())      // 6 - 3 - 2 + 2

	// Skip all buckets by jumping way ahead in time
	clock.SetTime(22500)
	window.ExpireBuckets()
	assert.Equal(t, int64(22), window.HeadTime)

	// All buckets should be reset
	for _, b := range window.Buckets {
		assert.Equal(t, int64(0), b.totalUsage)
		assert.Equal(t, uint32(0), b.samples)
	}
	assert.Equal(t, int64(0), window.TotalUsage())
	assert.Equal(t, uint32(0), window.Samples())

	// Record into bucket 2 after reset
	clock.SetTime(23100)
	recordUsage(window, []int64{500, 750, 250})
	assert.Equal(t, int64(23), window.HeadTime)
	assert.Equal(t, int64(1500), window.TotalUsage())
	assert.Equal(t, uint32(3), window.Samples())
}

func recordUsage(window *usageWindow, usages []int64) {
	for _, usage := range usages {
		window.RecordUsage(usage)
	}
}

func BenchmarkUsageTracker_Calibrate(b *testing.B) {
	userCounts := []int{10, 100, 1000, 10000}
	var minUsage int64 = 5
	var maxUsage int64 = 50
	rng := rand.New(rand.NewSource(42))

	for _, userCount := range userCounts {
		b.Run(fmt.Sprintf("users_%d", userCount), func(b *testing.B) {
			tracker := NewUsageTracker(time.Minute, userCount*2)

			for i := 0; i < userCount; i++ {
				userID := fmt.Sprintf("user_%d", i)
				usageRange := maxUsage - minUsage
				randomUsage := minUsage + rng.Int63n(usageRange)
				tracker.RecordUsage(userID, randomUsage)
			}

			// Reset timer to exclude setup time
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				tracker.Calibrate()
			}
		})
	}
}
