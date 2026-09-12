package interceptor

import (
	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

// Authorizer is implemented by both rbac.StaticAuthorizer and
// rbac.FileWatcherAuthorizer.
type Authorizer interface {
	IsAuthorized(ctx runtime.Ctx) bool
}

// RBAC returns a middleware.UnaryInterceptor that rejects requests not
// authorized by a with codes.CodePermissionDenied. Requests marked public
// by Public bypass authorization.
func RBAC(a Authorizer) middleware.UnaryInterceptor {
	return middleware.UnaryInterceptorFunc(func(next middleware.UnaryFunc) middleware.UnaryFunc {
		return func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
			if isPublic(ctx) {
				return next(ctx, req)
			}
			if !a.IsAuthorized(ctx) {
				return nil, newPermissionDenied("unauthorized RPC request rejected")
			}
			return next(ctx, req)
		}
	})
}

// RBACStream returns a middleware.StreamInterceptor that enforces the
// RBAC policy on the stream's transport context. Streams marked public by
// PublicStream bypass authorization.
func RBACStream(a Authorizer) middleware.StreamInterceptor {
	return middleware.StreamInterceptorFunc(func(next middleware.Stream) middleware.Stream {
		ctx := next.Context()
		if isPublic(ctx) {
			return next
		}
		if !a.IsAuthorized(ctx) {
			return &errorStream{ctx: ctx, err: newPermissionDenied("unauthorized RPC request rejected")}
		}
		return next
	})
}

func newPermissionDenied(format string, args ...any) error {
	return status.NewErrorf(codes.CodePermissionDenied, "authz: "+format, args...)
}
