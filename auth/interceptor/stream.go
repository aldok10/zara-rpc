package interceptor

import (
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/runtime"
)

// errorStream is a Stream whose Send/Receive always return err. It is
// returned by auth stream interceptors when the request is rejected, so
// the handler's first stream operation fails with the auth error.
type errorStream struct {
	ctx runtime.Ctx
	err error
}

func (s *errorStream) Context() runtime.Ctx  { return s.ctx }
func (s *errorStream) Send(any) error        { return s.err }
func (s *errorStream) Receive() (any, error) { return nil, s.err }

// claimsStream wraps a Stream and exposes a context carrying the auth
// claims, so handlers can read them via stream.Context().
type claimsStream struct {
	middleware.Stream
	ctx runtime.Ctx
}

func (s *claimsStream) Context() runtime.Ctx { return s.ctx }
