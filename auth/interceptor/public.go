package interceptor

import (
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/runtime"
)

// publicKey marks a Ctx as public (auth-skipped) in the interceptor context.
type publicKey struct{}

// Public returns a middleware.UnaryInterceptor that marks the Ctx as public
// when isPublic reports true, causing JWT and RBAC to skip authentication
// and authorization for the request. It is the interceptor-context
// replacement for router-level public-path filtering: the chain is
// [Public(isPublic), JWT(v), RBAC(a)] and the mux serves every route through
// one chain. isPublic reads the request from the Ctx (e.g. ctx.Spec().Path
// for HTTP, ctx.Spec().Procedure for gRPC).
func Public(isPublic func(ctx runtime.Ctx) bool) middleware.UnaryInterceptor {
	return middleware.UnaryInterceptorFunc(func(next middleware.UnaryFunc) middleware.UnaryFunc {
		return func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
			if isPublic(ctx) {
				ctx = ctx.WithValue(publicKey{}, true)
			}
			return next(ctx, req)
		}
	})
}

// PublicStream returns a middleware.StreamInterceptor that marks the stream
// context as public at stream establishment.
func PublicStream(isPublic func(ctx runtime.Ctx) bool) middleware.StreamInterceptor {
	return middleware.StreamInterceptorFunc(func(next middleware.Stream) middleware.Stream {
		ctx := next.Context()
		if isPublic(ctx) {
			ctx = ctx.WithValue(publicKey{}, true)
			return &publicStream{Stream: next, ctx: ctx}
		}
		return next
	})
}

// isPublic reports whether the Ctx was marked public by Public/PublicStream.
func isPublic(ctx runtime.Ctx) bool {
	v, _ := ctx.Value(publicKey{}).(bool)
	return v
}

// publicStream carries the public-marked Ctx through the stream.
type publicStream struct {
	middleware.Stream
	ctx runtime.Ctx
}

func (s *publicStream) Context() runtime.Ctx { return s.ctx }