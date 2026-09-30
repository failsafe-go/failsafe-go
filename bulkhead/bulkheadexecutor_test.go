package bulkhead

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/failsafe-go/failsafe-go"
)

func TestPermitListeners(t *testing.T) {
	for _, innerErr := range []error{nil, errors.New("execution failed")} {
		name := "success"
		if innerErr != nil {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			// Given
			var events []string
			var attempt failsafe.ExecutionAttempt[string]
			bh := NewBuilder[string](1).
				OnAcquired(func(e failsafe.ExecutionEvent[string]) {
					attempt = e.ExecutionAttempt
					events = append(events, "acquired")
				}).
				OnReleased(func(e failsafe.ExecutionEvent[string]) {
					assert.Same(t, attempt, e.ExecutionAttempt)
					events = append(events, "released")
				}).Build()

			// When
			result, err := failsafe.With(bh).Get(func() (string, error) {
				assert.False(t, bh.TryAcquirePermit())
				events = append(events, "execution")
				return "result", innerErr
			})

			// Then
			assert.Equal(t, "result", result)
			assert.Equal(t, innerErr, err)
			assert.Equal(t, []string{"acquired", "execution", "released"}, events)
			assert.True(t, bh.TryAcquirePermit())
			bh.ReleasePermit()
		})
	}
}

func TestPermitListenersWithoutAcquisition(t *testing.T) {
	for _, test := range []struct {
		name    string
		maxWait time.Duration
		cancel  bool
		wantErr error
		fulls   int
	}{
		{"full", 0, false, ErrFull, 1},
		{"max wait exceeded", 20 * time.Millisecond, false, ErrFull, 1},
		{"canceled while waiting", time.Second, true, context.Canceled, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Given
			acquired, released, fulls := 0, 0, 0
			bh := NewBuilder[any](1).WithMaxWaitTime(test.maxWait).
				OnAcquired(func(failsafe.ExecutionEvent[any]) { acquired++ }).
				OnReleased(func(failsafe.ExecutionEvent[any]) { released++ }).
				OnFull(func(failsafe.ExecutionEvent[any]) { fulls++ }).Build()
			require.True(t, bh.TryAcquirePermit())
			defer bh.ReleasePermit()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.cancel {
				timer := time.AfterFunc(20*time.Millisecond, cancel)
				defer timer.Stop()
			}

			// When
			err := failsafe.With(bh).WithContext(ctx).Run(func() error {
				t.Error("execution must not run without a permit")
				return nil
			})

			// Then
			assert.ErrorIs(t, err, test.wantErr)
			assert.Equal(t, test.fulls, fulls)
			assert.Zero(t, acquired)
			assert.Zero(t, released)
		})
	}
}

func TestPermitListenersConcurrentExecutions(t *testing.T) {
	// Given
	const executions = 16
	var acquired, released atomic.Int32
	bh := NewBuilder[any](executions).
		OnAcquired(func(failsafe.ExecutionEvent[any]) { acquired.Add(1) }).
		OnReleased(func(failsafe.ExecutionEvent[any]) { released.Add(1) }).Build()
	entered := make(chan struct{}, executions)
	finish := make(chan struct{})
	results := make(chan error, executions)

	// When
	for i := 0; i < executions; i++ {
		go func() {
			results <- failsafe.With(bh).Run(func() error {
				entered <- struct{}{}
				<-finish
				return nil
			})
		}()
	}
	for i := 0; i < executions; i++ {
		<-entered
	}
	assert.EqualValues(t, executions, acquired.Load())
	assert.Zero(t, released.Load())
	close(finish)

	// Then
	for i := 0; i < executions; i++ {
		assert.NoError(t, <-results)
	}
	assert.EqualValues(t, executions, acquired.Load())
	assert.Equal(t, acquired.Load(), released.Load())
}

func TestPermitListenersStandalone(t *testing.T) {
	listener := func(failsafe.ExecutionEvent[any]) {
		t.Error("standalone permit usage must not call execution listeners")
	}
	bh := NewBuilder[any](1).OnFull(listener).OnAcquired(listener).OnReleased(listener).Build()
	require.True(t, bh.TryAcquirePermit())
	assert.False(t, bh.TryAcquirePermit())
	bh.ReleasePermit()
	require.NoError(t, bh.AcquirePermit(nil))
	bh.ReleasePermit()
	require.NoError(t, bh.AcquirePermitWithMaxWait(nil, time.Second))
	assert.ErrorIs(t, bh.AcquirePermitWithMaxWait(nil, 0), ErrFull)
	bh.ReleasePermit()
}

func TestPermitListenersRegistration(t *testing.T) {
	for _, nilListeners := range []bool{false, true} {
		t.Run(map[bool]string{false: "last registration wins", true: "nil listeners"}[nilListeners], func(t *testing.T) {
			unexpected := func(failsafe.ExecutionEvent[any]) { t.Error("replaced listener called") }
			builder := NewBuilder[any](1).OnAcquired(unexpected).OnReleased(unexpected)
			acquired, released := 0, 0
			if nilListeners {
				builder.OnAcquired(nil).OnReleased(nil)
			} else {
				builder.OnAcquired(func(failsafe.ExecutionEvent[any]) { acquired++ }).
					OnReleased(func(failsafe.ExecutionEvent[any]) { released++ })
			}
			assert.NoError(t, failsafe.With(builder.Build()).Run(func() error { return nil }))
			if !nilListeners {
				assert.Equal(t, 1, acquired)
				assert.Equal(t, 1, released)
			}
		})
	}
}
