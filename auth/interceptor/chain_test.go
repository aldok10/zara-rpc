package interceptor

import (
	"context"
	"testing"

	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/runtime"
)

func TestChainInterceptors(t *testing.T) {
	var calls []string
	i1 := middleware.UnaryInterceptorFunc(func(next middleware.UnaryFunc) middleware.UnaryFunc {
		return func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
			calls = append(calls, "i1")
			return next(ctx, req)
		}
	})
	i2 := middleware.UnaryInterceptorFunc(func(next middleware.UnaryFunc) middleware.UnaryFunc {
		return func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
			calls = append(calls, "i2")
			return next(ctx, req)
		}
	})

	handler := func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
		calls = append(calls, "handler")
		return nil, nil
	}
	chain := middleware.ChainUnaryInterceptors([]middleware.UnaryInterceptor{i1, i2}, handler)
	ctx := runtime.NewCtx(context.Background(), metadata.RequestMeta{}, encoding.JSONCodec{})
	if _, err := chain(ctx, nil); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 3 || calls[0] != "i1" || calls[1] != "i2" || calls[2] != "handler" {
		t.Errorf("calls = %v, want [i1 i2 handler]", calls)
	}
}