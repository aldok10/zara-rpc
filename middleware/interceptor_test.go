package middleware

import (
	"io"
	"sync"
	"testing"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

// fakeStream is a minimal Stream implementation for unit tests.
type fakeStream struct {
	ctx      runtime.Ctx
	mu       sync.Mutex
	sent     []any
	received []any
	sendErr  error
	recvErr  error
}

func (f *fakeStream) Context() runtime.Ctx { return f.ctx }

func (f *fakeStream) Send(msg any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent = append(f.sent, msg)
	return nil
}

func (f *fakeStream) Receive() (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recvErr != nil {
		return nil, f.recvErr
	}
	if len(f.received) == 0 {
		return nil, io.EOF
	}
	msg := f.received[0]
	f.received = f.received[1:]
	return msg, nil
}

// recordingStreamInterceptor records every message that passes through and
// optionally rejects a message.
type recordingStreamInterceptor struct {
	mu       sync.Mutex
	observed []any
	reject   func(msg any) error
}

func (r *recordingStreamInterceptor) WrapStream(s Stream) Stream {
	return StreamInterceptorFunc(func(s Stream) Stream {
		return &interceptedFakeStream{
			inner: s,
			onSend: func(msg any) error {
				r.mu.Lock()
				r.observed = append(r.observed, msg)
				r.mu.Unlock()
				if r.reject != nil {
					return r.reject(msg)
				}
				return nil
			},
			onReceive: func() (any, error) {
				msg, err := s.Receive()
				if err != nil {
					return nil, err
				}
				r.mu.Lock()
				r.observed = append(r.observed, msg)
				r.mu.Unlock()
				return msg, nil
			},
		}
	})(s)
}

type interceptedFakeStream struct {
	inner     Stream
	onSend    func(any) error
	onReceive func() (any, error)
}

func (s *interceptedFakeStream) Context() runtime.Ctx { return s.inner.Context() }

// Send observes via onSend, then forwards to inner. onSend must NOT forward
// itself; forwarding happens here so each message crosses the wire exactly
// once.
func (s *interceptedFakeStream) Send(msg any) error {
	if err := s.onSend(msg); err != nil {
		return err
	}
	return s.inner.Send(msg)
}

func (s *interceptedFakeStream) Receive() (any, error) {
	return s.onReceive()
}

func (r *recordingStreamInterceptor) observedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.observed)
}

func (r *recordingStreamInterceptor) observedMessages() []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]any, len(r.observed))
	copy(out, r.observed)
	return out
}

// TestChainStreamInterceptorsOrdering verifies that interceptors compose in
// registration order: the first interceptor is the outermost wrapper.
func TestChainStreamInterceptorsOrdering(t *testing.T) {
	var order []string
	first := StreamInterceptorFunc(func(s Stream) Stream {
		order = append(order, "first:wrap")
		return &interceptedFakeStream{
			inner: s,
			onSend: func(msg any) error {
				order = append(order, "first:send")
				return nil
			},
			onReceive: func() (any, error) {
				order = append(order, "first:recv")
				return s.Receive()
			},
		}
	})
	second := StreamInterceptorFunc(func(s Stream) Stream {
		order = append(order, "second:wrap")
		return &interceptedFakeStream{
			inner: s,
			onSend: func(msg any) error {
				order = append(order, "second:send")
				return nil
			},
			onReceive: func() (any, error) {
				order = append(order, "second:recv")
				return s.Receive()
			},
		}
	})

	inner := &fakeStream{}
	chained := ChainStreamInterceptors([]StreamInterceptor{first, second}, inner)

	if err := chained.Send("hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, err := chained.Receive(); err != io.EOF {
		t.Fatalf("Receive: %v", err)
	}

	// The chain builds inside-out (second wraps first), but the first
	// interceptor is the outermost: sends and receives flow first -> second
	// -> inner.
	want := []string{
		"second:wrap", "first:wrap",
		"first:send", "second:send",
		"first:recv", "second:recv",
	}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

// TestStreamInterceptorInspection verifies that an interceptor observes
// every message before it reaches the wire.
func TestStreamInterceptorInspection(t *testing.T) {
	rec := &recordingStreamInterceptor{}
	inner := &fakeStream{}
	chained := ChainStreamInterceptors([]StreamInterceptor{rec}, inner)

	if err := chained.Send("msg1"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := chained.Send("msg2"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := rec.observedMessages(); len(got) != 2 || got[0] != "msg1" || got[1] != "msg2" {
		t.Fatalf("observed = %v, want [msg1 msg2]", got)
	}
}

// TestStreamInterceptorRejection verifies that an interceptor error
// terminates the stream with the mapped RPC status.
func TestStreamInterceptorRejection(t *testing.T) {
	rec := &recordingStreamInterceptor{
		reject: func(msg any) error {
			return status.NewErrorf(codes.CodePermissionDenied, "message %v rejected", msg)
		},
	}
	inner := &fakeStream{}
	chained := ChainStreamInterceptors([]StreamInterceptor{rec}, inner)

	err := chained.Send("blocked")
	if err == nil {
		t.Fatal("Send: expected rejection error")
	}
	if got := status.Code(err); got != codes.CodePermissionDenied {
		t.Fatalf("Code = %v, want %v", got, codes.CodePermissionDenied)
	}
	// The rejected message must not reach the wire.
	if got := inner.sent; len(got) != 0 {
		t.Fatalf("inner.sent = %v, want none", got)
	}
}

// TestStreamInterceptorSSEIntegration runs a full server-streaming endpoint
// over SSE with a stream interceptor that inspects and then rejects a
// message, verifying the stream stops after the rejection.
func TestStreamInterceptorErrorPropagation(t *testing.T) {
	rec := &recordingStreamInterceptor{
		reject: func(msg any) error {
			return status.NewErrorf(codes.CodeAborted, "abort stream")
		},
	}
	inner := &fakeStream{}
	chained := ChainStreamInterceptors([]StreamInterceptor{rec}, inner)

	err := chained.Send("x")
	if got := status.Code(err); got != codes.CodeAborted {
		t.Fatalf("Code = %v, want %v", got, codes.CodeAborted)
	}
}
