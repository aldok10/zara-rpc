// Package middleware holds the interceptor chains and auth layer.
package middleware

import "github.com/aldok10/zara-rpc/runtime"

// UnaryFunc is the signature of a unary RPC handler after interception.
// The ctx is a runtime.Ctx: unary interceptors can read transport metadata
// and the typed request payload (ctx.Request[T]()) and set the response
// payload (ctx.SetResponse) without type assertions.
type UnaryFunc func(runtime.Ctx, runtime.AnyRequest) (runtime.AnyResponse, error)

// UnaryInterceptor wraps a UnaryFunc. Unary interceptors are composed like
// an onion: the first interceptor in the list is the outermost.
type UnaryInterceptor interface {
	WrapUnary(UnaryFunc) UnaryFunc
}

// UnaryInterceptorFunc adapts a plain function to the UnaryInterceptor
// interface.
type UnaryInterceptorFunc func(UnaryFunc) UnaryFunc

// WrapUnary implements UnaryInterceptor.
func (f UnaryInterceptorFunc) WrapUnary(next UnaryFunc) UnaryFunc {
	return f(next)
}

// ChainUnaryInterceptors composes unary interceptors in order. The first
// interceptor is the outermost wrapper. It is exported so callers can
// reuse the same chain outside the mux (e.g. on a gRPC adapter path).
func ChainUnaryInterceptors(interceptors []UnaryInterceptor, next UnaryFunc) UnaryFunc {
	chained := next
	for i := len(interceptors) - 1; i >= 0; i-- {
		chained = interceptors[i].WrapUnary(chained)
	}
	return chained
}

// Stream is the type-erased stream view passed to stream interceptors.
// Send writes one message to the peer; Receive reads the next message and
// returns io.EOF when the peer has finished sending. Unidirectional
// transports return an error from the unused direction. Context returns the
// transport Ctx the stream was established with, so interceptors can read
// metadata (headers, query, cookies) and attach values (e.g. auth claims)
// that handlers observe via stream.Context().
type Stream interface {
	Context() runtime.Ctx
	Send(any) error
	Receive() (any, error)
}

// StreamInterceptor wraps a Stream. Unlike UnaryInterceptor, which wraps
// the handler, a StreamInterceptor wraps the stream object itself so each
// Send/Receive is observed per message. Stream interceptors are composed
// like an onion: the first interceptor in the list is the outermost.
type StreamInterceptor interface {
	WrapStream(Stream) Stream
}

// StreamInterceptorFunc adapts a plain function to the StreamInterceptor
// interface.
type StreamInterceptorFunc func(Stream) Stream

// WrapStream implements StreamInterceptor.
func (f StreamInterceptorFunc) WrapStream(s Stream) Stream {
	return f(s)
}

// ChainStreamInterceptors composes stream interceptors in order. The first
// interceptor is the outermost wrapper. It is exported so callers can
// reuse the same chain outside the mux.
func ChainStreamInterceptors(interceptors []StreamInterceptor, next Stream) Stream {
	chained := next
	for i := len(interceptors) - 1; i >= 0; i-- {
		chained = interceptors[i].WrapStream(chained)
	}
	return chained
}
