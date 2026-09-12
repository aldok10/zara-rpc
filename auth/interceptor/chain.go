package interceptor

import (
	"github.com/aldok10/zara-rpc/middleware"
)

// Chain composes unary interceptors into one. Interceptors execute in
// the order provided: the first wraps the second, which wraps the third,
// and so on.
func Chain(unis ...middleware.UnaryInterceptor) middleware.UnaryInterceptor {
	return middleware.UnaryInterceptorFunc(func(next middleware.UnaryFunc) middleware.UnaryFunc {
		return middleware.ChainUnaryInterceptors(unis, next)
	})
}

// ChainStream composes stream interceptors into one.
func ChainStream(strs ...middleware.StreamInterceptor) middleware.StreamInterceptor {
	return middleware.StreamInterceptorFunc(func(next middleware.Stream) middleware.Stream {
		return middleware.ChainStreamInterceptors(strs, next)
	})
}
