package status

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aldok10/zara-rpc/codes"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestErrorUnwrap(t *testing.T) {
	base := errors.New("boom")
	rpcErr := NewError(codes.CodeInternal, base)
	if !errors.Is(rpcErr, base) {
		t.Error("errors.Is should find the wrapped error")
	}
	if Code(rpcErr) != codes.CodeInternal {
		t.Errorf("Code = %v, want internal", Code(rpcErr))
	}
}

func TestErrorCodeOfUnknown(t *testing.T) {
	if Code(errors.New("plain")) != codes.CodeUnknown {
		t.Error("Code(plain error) should be unknown")
	}
}

func TestErrorMeta(t *testing.T) {
	rpcErr := NewErrorf(codes.CodeUnauthenticated, "token expired")
	rpcErr.Meta().Set("WWW-Authenticate", "Bearer")
	if got := rpcErr.Meta().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("meta = %q, want Bearer", got)
	}
}

func TestErrorMessage(t *testing.T) {
	rpcErr := NewErrorf(codes.CodeNotFound, "user %q not found", "42")
	if got := rpcErr.Message(); got != `user "42" not found` {
		t.Errorf("Message() = %q", got)
	}
	if got := rpcErr.Error(); got == "" {
		t.Error("Error() should not be empty")
	}
}

// fakeGRPCStatus mirrors the method shape of grpc's *status.Status without
// importing grpc (the root module is grpc-free). FromGRPCStatus finds these
// methods by name via reflection. The code type is uint32, matching grpc's
// codes.Code — a signed fake would not catch the reflect.Value.Int panic
// on unsigned values.
type fakeGRPCStatus struct {
	code    uint32
	message string
}

func (s *fakeGRPCStatus) Code() uint32    { return s.code }
func (s *fakeGRPCStatus) Message() string { return s.message }

type fakeGRPCError struct {
	st *fakeGRPCStatus
}

func (e *fakeGRPCError) Error() string               { return "fake grpc error" }
func (e *fakeGRPCError) GRPCStatus() *fakeGRPCStatus { return e.st }

func TestFromGRPCStatus(t *testing.T) {
	t.Run("converts grpc status error", func(t *testing.T) {
		err := &fakeGRPCError{st: &fakeGRPCStatus{code: 5, message: "user missing"}}
		got := FromGRPCStatus(err)
		if Code(got) != codes.CodeNotFound {
			t.Errorf("Code = %v, want not_found", Code(got))
		}
		if got.Error() != "rpc error: not_found: user missing" {
			t.Errorf("Error() = %q", got.Error())
		}
	})

	t.Run("nil stays nil", func(t *testing.T) {
		if got := FromGRPCStatus(nil); got != nil {
			t.Errorf("FromGRPCStatus(nil) = %v, want nil", got)
		}
	})

	t.Run("non-grpc error unchanged", func(t *testing.T) {
		plain := errors.New("plain")
		if got := FromGRPCStatus(plain); got != plain {
			t.Errorf("FromGRPCStatus(plain) = %v, want the same error", got)
		}
	})

	t.Run("unwraps wrapped grpc error", func(t *testing.T) {
		inner := &fakeGRPCError{st: &fakeGRPCStatus{code: 13, message: "boom"}}
		wrapped := fmt.Errorf("outer: %w", inner)
		got := FromGRPCStatus(wrapped)
		if Code(got) != codes.CodeInternal {
			t.Errorf("Code = %v, want internal", Code(got))
		}
	})
}
