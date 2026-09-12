package kernel

import (
	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

// typedServerStream adapts a runtime.ServerStream[any] to
// runtime.ServerStream[Res].
type typedServerStream[Res any] struct {
	inner runtime.ServerStream[any]
}

func (s *typedServerStream[Res]) Send(msg Res) error {
	return s.inner.Send(msg)
}

// receiveTyped reads the next message from a type-erased stream and
// asserts it to T. It is shared by the typed client and bidi adapters so
// the type-assertion error path is defined once.
func receiveTyped[T any](inner interface{ Receive() (any, error) }) (T, error) {
	var zero T
	msg, err := inner.Receive()
	if err != nil {
		return zero, err
	}
	typed, ok := msg.(T)
	if !ok {
		return zero, status.NewErrorf(codes.CodeInternal, "unexpected message type %T", msg)
	}
	return typed, nil
}

// typedClientStream adapts a runtime.ClientStream[any] to
// runtime.ClientStream[Req].
type typedClientStream[Req any] struct {
	inner runtime.ClientStream[any]
}

func (s *typedClientStream[Req]) Receive() (Req, error) {
	return receiveTyped[Req](s.inner)
}

// typedBidiStream adapts a runtime.BidiStream[any, any] to
// runtime.BidiStream[Req, Res].
type typedBidiStream[Req, Res any] struct {
	inner runtime.BidiStream[any, any]
}

func (s *typedBidiStream[Req, Res]) Send(msg Res) error {
	return s.inner.Send(msg)
}

func (s *typedBidiStream[Req, Res]) Receive() (Req, error) {
	return receiveTyped[Req](s.inner)
}
