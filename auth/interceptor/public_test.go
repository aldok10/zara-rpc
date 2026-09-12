package interceptor

import (
	"context"
	"testing"

	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/runtime"
)

func TestPublicInterceptor(t *testing.T) {
	isPublic := func(ctx runtime.Ctx) bool {
		return ctx.Spec().Path == "/v1/auth/login"
	}
	interceptor := Public(isPublic)

	next := func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
		if !isPublic(ctx) {
			t.Error("isPublic(ctx) = false, want true for public path")
		}
		return nil, nil
	}
	wrapped := interceptor.WrapUnary(next)

	meta := metadata.RequestMeta{}
	ctx := runtime.NewCtx(context.Background(), meta, encoding.JSONCodec{}).
		WithSpec(runtime.Spec{Path: "/v1/auth/login"})

	if _, err := wrapped(ctx, nil); err != nil {
		t.Fatalf("wrapped = %v, want nil", err)
	}
}