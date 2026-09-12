// Package streaming adapts transports to the type-erased middleware.Stream
// view and holds the stream transport registry.
package streaming

import (
	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

// streamServerAdapter adapts a send-only runtime.ServerStream[any] to the
// type-erased middleware.Stream view used by stream interceptors. Receive
// returns an error because server-streaming transports cannot receive.
type streamServerAdapter struct {
	ctx   runtime.Ctx
	inner runtime.ServerStream[any]
}

func (s *streamServerAdapter) Context() runtime.Ctx { return s.ctx }

func (s *streamServerAdapter) Send(msg any) error { return s.inner.Send(msg) }

func (s *streamServerAdapter) Receive() (any, error) {
	return nil, status.NewErrorf(codes.CodeInternal, "receive not supported on server-streaming transport")
}

// streamClientAdapter adapts a receive-only runtime.ClientStream[any] to
// middleware.Stream. Send returns an error because client-streaming
// transports cannot send.
type streamClientAdapter struct {
	ctx   runtime.Ctx
	inner runtime.ClientStream[any]
}

func (s *streamClientAdapter) Context() runtime.Ctx { return s.ctx }

func (s *streamClientAdapter) Send(any) error {
	return status.NewErrorf(codes.CodeInternal, "send not supported on client-streaming transport")
}

func (s *streamClientAdapter) Receive() (any, error) { return s.inner.Receive() }

// streamBidiAdapter adapts a bidirectional stream to the type-erased
// middleware.Stream view, carrying the transport Ctx. The transport
// package's WSBidiStream cannot import kernel (import cycle), so the
// adapter lives here.
type streamBidiAdapter struct {
	ctx   runtime.Ctx
	inner runtime.BidiStream[any, any]
}

func (s *streamBidiAdapter) Context() runtime.Ctx { return s.ctx }

func (s *streamBidiAdapter) Send(msg any) error { return s.inner.Send(msg) }

func (s *streamBidiAdapter) Receive() (any, error) { return s.inner.Receive() }

// NewServerStreamAdapter wraps a server stream for the interceptor chain.
func NewServerStreamAdapter(ctx runtime.Ctx, inner runtime.ServerStream[any]) middleware.Stream {
	return &streamServerAdapter{ctx: ctx, inner: inner}
}

// NewClientStreamAdapter wraps a client stream for the interceptor chain.
func NewClientStreamAdapter(ctx runtime.Ctx, inner runtime.ClientStream[any]) middleware.Stream {
	return &streamClientAdapter{ctx: ctx, inner: inner}
}

// NewBidiStreamAdapter wraps a bidi stream for the interceptor chain.
func NewBidiStreamAdapter(ctx runtime.Ctx, inner runtime.BidiStream[any, any]) middleware.Stream {
	return &streamBidiAdapter{ctx: ctx, inner: inner}
}