// Client-side interceptor chain, mirroring the server model in runtime.
package client

import (
	"context"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/status"
)

// UnaryInvoker performs a single unary RPC attempt. body is the encoded
// request body (nil for bodyless calls); it is passed as []byte so retries
// can re-send the exact same bytes without re-encoding. The invoker returns
// the response body byte count (for observability hooks) and the RPC error.
type UnaryInvoker func(ctx context.Context, method, path string, body []byte, req, resp any, cfg *clientConfig) (int64, error)

// UnaryInterceptor wraps a UnaryInvoker. Unary interceptors are composed
// like an onion: the first interceptor in the list is the outermost.
type UnaryInterceptor interface {
	WrapUnary(UnaryInvoker) UnaryInvoker
}

// UnaryInterceptorFunc adapts a plain function to the UnaryInterceptor
// interface.
type UnaryInterceptorFunc func(UnaryInvoker) UnaryInvoker

// WrapUnary implements UnaryInterceptor.
func (f UnaryInterceptorFunc) WrapUnary(next UnaryInvoker) UnaryInvoker {
	return f(next)
}

// ChainUnaryInterceptors composes unary interceptors in order. The first
// interceptor is the outermost wrapper.
func ChainUnaryInterceptors(interceptors []UnaryInterceptor, next UnaryInvoker) UnaryInvoker {
	chained := next
	for i := len(interceptors) - 1; i >= 0; i-- {
		chained = interceptors[i].WrapUnary(chained)
	}
	return chained
}

// codeOf maps an error to an RPC code for observability hooks. Plain
// context errors map to their gRPC equivalents; everything else falls back
// to status.Code.
func codeOf(err error) codes.Code {
	if err == nil {
		return codes.CodeOK
	}
	if isContextErr(err, context.DeadlineExceeded) {
		return codes.CodeDeadlineExceeded
	}
	if isContextErr(err, context.Canceled) {
		return codes.CodeCanceled
	}
	return status.Code(err)
}

func isContextErr(err, target error) bool {
	for e := err; e != nil; e = unwrap(e) {
		if e == target {
			return true
		}
	}
	return false
}

func unwrap(err error) error {
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return u.Unwrap()
	}
	return nil
}