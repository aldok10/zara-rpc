package interceptor

import (
	"github.com/aldok10/zara-rpc/auth/jwt"
	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

// JWT returns a middleware.UnaryInterceptor that authenticates every request
// using v: extracts the token, validates it, attaches the parsed claims
// to the Ctx (via jwt.WithClaims), and rejects unauthenticated requests.
// Requests marked public by Public bypass validation.
func JWT(v *jwt.JWTValidator) middleware.UnaryInterceptor {
	return middleware.UnaryInterceptorFunc(func(next middleware.UnaryFunc) middleware.UnaryFunc {
		return func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
			if isPublic(ctx) {
				return next(ctx, req)
			}
			token, ok := v.ExtractToken(ctx)
			if !ok {
				return nil, newUnauthenticated("missing bearer token")
			}
			claims, err := v.Validate(token)
			if err != nil {
				return nil, newUnauthenticated("invalid token: %v", err)
			}
			return next(jwt.WithClaims(ctx, claims), req)
		}
	})
}

// JWTStream returns a middleware.StreamInterceptor that validates the JWT at
// stream establishment and attaches the parsed claims to the stream context.
// Streams marked public by PublicStream bypass validation.
func JWTStream(v *jwt.JWTValidator) middleware.StreamInterceptor {
	return middleware.StreamInterceptorFunc(func(next middleware.Stream) middleware.Stream {
		ctx := next.Context()
		if isPublic(ctx) {
			return next
		}
		token, ok := v.ExtractToken(ctx)
		if !ok {
			return &errorStream{ctx: ctx, err: newUnauthenticated("missing bearer token")}
		}
		claims, err := v.Validate(token)
		if err != nil {
			return &errorStream{ctx: ctx, err: newUnauthenticated("invalid token: %v", err)}
		}
		return &claimsStream{Stream: next, ctx: jwt.WithClaims(ctx, claims)}
	})
}

func newUnauthenticated(format string, args ...any) error {
	return status.NewErrorf(codes.CodeUnauthenticated, "auth: "+format, args...)
}
