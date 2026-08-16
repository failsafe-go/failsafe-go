package issues

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/failsafegrpc"
)

// See https://github.com/failsafe-go/failsafe-go/issues/136
// Asserts that concurrent calls through an interceptor do not race on a shared Executor. Requires -race.
func TestIssue136_concurrentCalls(t *testing.T) {
	// Given
	cb := circuitbreaker.NewBuilder[any]().Build()
	interceptor := failsafegrpc.NewUnaryClientInterceptorWithExecutor[any](failsafe.With[any](cb))
	mockInvoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return nil
	}

	// When performing concurrent calls, each with a distinct request scoped context
	type customKey int
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx := context.WithValue(context.Background(), customKey(i), "foo")
			assert.NoError(t, interceptor(ctx, "/test.Method", nil, nil, nil, mockInvoker))
		}(i)
	}

	// Then
	wg.Wait()
}
