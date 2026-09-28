package util

import (
	"bytes"
	"context"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeContextsStopsWatcherOnCancel(t *testing.T) {
	for _, cancellableParents := range []bool{true, false} {
		name := "long-lived cancellable parents"
		if !cancellableParents {
			name = "parents with nil Done channels"
		}
		t.Run(name, func(t *testing.T) {
			var ctx1, ctx2 context.Context
			if cancellableParents {
				var cancel1, cancel2 context.CancelFunc
				ctx1, cancel1 = context.WithCancel(context.Background())
				ctx2, cancel2 = context.WithCancel(context.Background())
				defer cancel1()
				defer cancel2()
			} else {
				type key int
				ctx1 = context.WithValue(context.Background(), key(1), "first")
				ctx2 = context.WithValue(context.Background(), key(2), "second")
			}

			before := mergeContextWatcherCount(t)
			for i := 0; i < 16; i++ {
				merged, cancel := MergeContexts(ctx1, ctx2)
				cancel(nil)
				require.ErrorIs(t, merged.Err(), context.Canceled)
			}

			// Count this helper's watchers rather than unrelated runtime or test
			// goroutines. Parents stay alive until after the assertion.
			after := mergeContextWatcherCount(t)
			deadline := time.Now().Add(time.Second)
			for after > before && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
				after = mergeContextWatcherCount(t)
			}
			assert.LessOrEqual(t, after, before, "watchers should stop when the merged context is canceled")
			require.NoError(t, ctx1.Err())
			require.NoError(t, ctx2.Err())
		})
	}
}

func mergeContextWatcherCount(t *testing.T) int {
	t.Helper()
	var stacks bytes.Buffer
	require.NoError(t, pprof.Lookup("goroutine").WriteTo(&stacks, 2))
	return strings.Count(stacks.String(), "util.MergeContexts.func1(")
}
