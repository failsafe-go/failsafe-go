package priority

import (
	"container/list"
	"context"
	"sort"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go/internal/util"
)

// UserKey is a key to use with a Context that stores a user ID.
const UserKey key = 2

// ContextWithUser returns a context with the userID stored with the UserKey.
func ContextWithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserKey, userID)
}

// UserFromContext returns the userID from the context, else "".
func UserFromContext(ctx context.Context) string {
	if untypedUser := ctx.Value(UserKey); untypedUser != nil {
		if userID, ok := untypedUser.(string); ok {
			return userID
		}
	}
	return ""
}

// UsageTracker tracks resource usage per user as execution usages for fair execution prioritization.
type UsageTracker interface {
	// RecordUsage calculates and records usage for the user.
	RecordUsage(userID string, usage int64)

	// GetUsage returns the total recorded usage for a user.
	GetUsage(userID string) int64

	// GetLevel returns the priority level for a user based on their recent usage.
	GetLevel(userID string, priority Priority) int

	// Calibrate calibrates levels based on the distribution of usage across all users.
	Calibrate()
}

type usageTracker struct {
	maxUsers    int
	newWindowFn func() *util.UsageWindow

	mu sync.RWMutex
	// Guarded by mu
	users map[string]*userEntry
	lru   *list.List
}

type userEntry struct {
	window     *util.UsageWindow
	quantile   float64 // Negative value represents being uncalibrated
	lruElement *list.Element
}

// NewUsageTracker creates a new UsageTracker with the specified configuration.
func NewUsageTracker(maxUsers int, windowDuration time.Duration) UsageTracker {
	return &usageTracker{
		maxUsers: maxUsers,
		newWindowFn: func() *util.UsageWindow {
			return util.NewUsageWindow(30, windowDuration, util.WallClock)
		},
		users: make(map[string]*userEntry),
		lru:   list.New(),
	}
}

func (tt *usageTracker) RecordUsage(userID string, usage int64) {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	entry := tt.users[userID]
	if entry == nil {
		if len(tt.users) >= tt.maxUsers {
			tt.evictOldest()
		}
		entry = &userEntry{
			window:   tt.newWindowFn(),
			quantile: -1,
		}
		tt.users[userID] = entry
		entry.lruElement = tt.lru.PushFront(userID)
	} else {
		tt.lru.MoveToFront(entry.lruElement)
	}

	entry.window.RecordUsage(usage)
}

func (tt *usageTracker) GetLevel(userID string, priority Priority) int {
	tt.mu.RLock()
	entry := tt.users[userID]
	tt.mu.RUnlock()

	lRange := priority.levelRange()
	// Handle users that have no recorded usages
	if entry == nil {
		return lRange.upper
	}

	// Handle new entries with no recently recorded usages
	totalUsage := entry.window.TotalUsage()
	if totalUsage == 0 {
		return lRange.upper
	}

	// Handle uncalibrated user
	if entry.quantile < 0 {
		return lRange.upper
	}

	fairnessScore := 1.0 - entry.quantile
	return lRange.lower + int(99.0*fairnessScore)
}

func (tt *usageTracker) GetUsage(userID string) int64 {
	tt.mu.RLock()
	defer tt.mu.RUnlock()

	entry := tt.users[userID]
	if entry == nil {
		return 0
	}
	return entry.window.TotalUsage()
}

// Calibrate has an O(n log n) time complexity, where n is the number of userse.
func (tt *usageTracker) Calibrate() {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	usages := make([]int64, 0, len(tt.users))
	for _, entry := range tt.users {
		entry.window.ExpireBuckets()
		if usage := entry.window.TotalUsage(); usage > 0 {
			usages = append(usages, usage)
		}
	}

	sort.Slice(usages, func(i, j int) bool {
		return usages[i] < usages[j]
	})

	// Update percentiles for all active users
	for _, entry := range tt.users {
		if usage := entry.window.TotalUsage(); usage > 0 {
			entry.quantile = tt.computeQuantile(usage, usages)
		} else {
			entry.quantile = -1
		}
	}
}

// computeQuantile returns the quantile for a usage, among the sortedUsages.
func (tt *usageTracker) computeQuantile(usage int64, sortedUsages []int64) float64 {
	if len(sortedUsages) == 0 {
		return 0
	}

	index := sort.Search(len(sortedUsages), func(i int) bool {
		return sortedUsages[i] >= usage
	})

	return util.Round(float64(index) / float64(len(sortedUsages)))
}

func (tt *usageTracker) evictOldest() {
	if oldest := tt.lru.Back(); oldest != nil {
		userID := oldest.Value.(string)
		delete(tt.users, userID)
		tt.lru.Remove(oldest)
	}
}
