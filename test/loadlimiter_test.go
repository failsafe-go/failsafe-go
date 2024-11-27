package test

//func TestFailureSignal(t *testing.T) {
//	ll := loadlimiter.New[any](loadlimiter.NewFailureSignal(), loadlimiter.NewAdaptiveThrottlingStrategy())
//
//	testutil.Test[any](t).
//		With(ll).
//		Get(func(execution failsafe.Execution[any]) (any, error) {
//			return nil, nil
//		})
//}
