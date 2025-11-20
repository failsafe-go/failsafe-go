package issues

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/failsafegrpc"
	"github.com/failsafe-go/failsafe-go/priority"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// See https://github.com/failsafe-go/failsafe-go/issues/122
func TestIssue122_bug1(t *testing.T) {
	// Given
	var executionCtx context.Context // Capture ctx during execution
	rp := retrypolicy.NewBuilder[any]().
		OnSuccess(func(e failsafe.ExecutionEvent[any]) {
			executionCtx = e.Context()
		}).Build()
	executor := failsafe.With[any](rp)
	interceptor := failsafegrpc.NewUnaryClientInterceptorWithExecutor[any](executor)
	mockInvoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return nil
	}

	// When
	err := interceptor(priority.High.AddTo(context.Background()), "/test.Method", nil, nil, nil, mockInvoker)
	assert.NoError(t, err)

	// Then
	assert.Equal(t, priority.High, priority.FromContext(executionCtx))
}

func TestIssue122_bug2(t *testing.T) {
	// Given
	var executionCtx context.Context // Capture ctx during execution
	rp := retrypolicy.NewBuilder[any]().
		OnSuccess(func(e failsafe.ExecutionEvent[any]) {
			executionCtx = e.Context()
		}).Build()

	type customKey int
	const testKey customKey = 0
	executor := failsafe.With[any](rp).WithContext(context.WithValue(context.Background(), testKey, "foo"))
	interceptor := failsafegrpc.NewUnaryClientInterceptorWithExecutor[any](executor)
	mockInvoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return nil
	}

	// When
	err := interceptor(priority.High.AddTo(context.Background()), "/test.Method", nil, nil, nil, mockInvoker)
	assert.NoError(t, err)

	// Then
	assert.Equal(t, "foo", executionCtx.Value(testKey))
	assert.Equal(t, priority.High, priority.FromContext(executionCtx))
}
