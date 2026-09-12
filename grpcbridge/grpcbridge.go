// Package grpcbridge bridges the zararpc runtime and grpc-go. It converts
// incoming gRPC contexts into runtime.Ctx (so one handler implementation
// serves both HTTP and gRPC), converts zararpc errors to gRPC status errors,
// and adapts grpc.ServerStream to the runtime stream interfaces.
//
// This is the only grpc-importing package in the framework root module: the
// core packages (runtime, client, codes, status, ...) stay grpc-free. The
// generated .grpc.go files (RegisterXxxServiceServer) import this package
// instead of emitting a per-package bridge, so generated output stays thin.
package grpcbridge

import (
	"context"
	"net/http"

	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/routing"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

// UnaryInterceptor bridges zararpc middleware.UnaryInterceptor chain to a grpc.UnaryServerInterceptor.
func UnaryInterceptor(interceptors ...middleware.UnaryInterceptor) grpc.ServerOption {
	if len(interceptors) == 0 {
		return grpc.EmptyServerOption{}
	}
	fn := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		rctx := CtxFor(ctx, info.FullMethod)
		var resp any
		wrapped := middleware.ChainUnaryInterceptors(interceptors, func(ctx runtime.Ctx, _ runtime.AnyRequest) (runtime.AnyResponse, error) {
			r, err := handler(ctx.Context(), req)
			resp = r
			return nil, err
		})
		if _, err := wrapped(rctx, nil); err != nil {
			return nil, ToGRPC(err)
		}
		return resp, nil
	}
	return grpc.UnaryInterceptor(fn)
}

// StreamInterceptor bridges zararpc middleware.UnaryInterceptor chain to a grpc.StreamServerInterceptor
// so auth/public interceptors run before stream establishment over gRPC.
func StreamInterceptor(interceptors ...middleware.UnaryInterceptor) grpc.ServerOption {
	if len(interceptors) == 0 {
		return grpc.EmptyServerOption{}
	}
	fn := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		rctx := CtxFor(ss.Context(), info.FullMethod)
		wrapped := middleware.ChainUnaryInterceptors(interceptors, func(ctx runtime.Ctx, _ runtime.AnyRequest) (runtime.AnyResponse, error) {
			return nil, nil
		})
		if _, err := wrapped(rctx, nil); err != nil {
			return ToGRPC(err)
		}
		return handler(srv, ss)
	}
	return grpc.StreamInterceptor(fn)
}

func init() {
	routing.DefaultGRPCFactory = func(unary []middleware.UnaryInterceptor, stream []middleware.StreamInterceptor) (http.Handler, any) {
		var opts []grpc.ServerOption
		if len(unary) > 0 {
			opts = append(opts, UnaryInterceptor(unary...), StreamInterceptor(unary...))
		}
		srv := grpc.NewServer(opts...)
		reflection.Register(srv)
		return srv, srv
	}
}

// WithGRPC enables gRPC support on routing.Server. It automatically receives
// the unary and stream interceptors configured on the Server via
// routing.WithUnaryInterceptors / routing.WithStreamInterceptors, applies them
// to the gRPC server, and registers gRPC reflection. Extra grpc.ServerOptions
// can be passed if needed.
func WithGRPC(extraOpts ...grpc.ServerOption) routing.ServerOption {
	return routing.WithGRPCFactory(func(unary []middleware.UnaryInterceptor, stream []middleware.StreamInterceptor) (http.Handler, any) {
		var opts []grpc.ServerOption
		if len(unary) > 0 {
			opts = append(opts, UnaryInterceptor(unary...), StreamInterceptor(unary...))
		}
		opts = append(opts, extraOpts...)
		srv := grpc.NewServer(opts...)
		reflection.Register(srv)
		return srv, srv
	})
}

// CtxFor converts an incoming gRPC context into a runtime.Ctx so handlers
// see the same header API over both transports. The procedure spec is set
// so RBAC policies can match on it (ctx.Spec().Procedure).
func CtxFor(ctx context.Context, procedure string) runtime.Ctx {
	md, _ := grpcmetadata.FromIncomingContext(ctx)
	header := make(http.Header)
	for k, vs := range md {
		for _, v := range vs {
			header.Add(k, v)
		}
	}
	return runtime.NewCtx(ctx, metadata.RequestMeta{Header: header}, encoding.ProtoCodec{}).
		WithProtocol("grpc").
		WithSpec(runtime.Spec{Procedure: procedure})
}

// ToGRPC converts a zararpc error to a grpc status error so error codes and
// error details survive the gRPC hop. Errors that already are gRPC status
// errors pass through unchanged.
func ToGRPC(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := grpcstatus.FromError(err); ok {
		return err
	}
	st := &spb.Status{
		Code:    int32(status.Code(err)),
		Message: err.Error(),
	}
	if rpcErr, ok := status.FromError(err); ok {
		st.Details = rpcErr.Details()
	}
	return grpcstatus.FromProto(st).Err()
}

// ServerStreamAdapter adapts a grpc.ServerStream to runtime.ServerStream[*T].
type ServerStreamAdapter[T any] struct {
	Stream grpc.ServerStream
}

// Send forwards one message to the client.
func (a *ServerStreamAdapter[T]) Send(m *T) error { return a.Stream.SendMsg(m) }

// ClientStreamAdapter adapts a grpc.ServerStream to runtime.ClientStream[*T].
type ClientStreamAdapter[T any] struct {
	Stream grpc.ServerStream
}

// Receive reads one message from the client.
func (a *ClientStreamAdapter[T]) Receive() (*T, error) {
	m := new(T)
	if err := a.Stream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// BidiStreamAdapter adapts a grpc.ServerStream to runtime.BidiStream[*Req, *Resp].
type BidiStreamAdapter[Req, Resp any] struct {
	Stream grpc.ServerStream
}

// Send forwards one response message to the client.
func (a *BidiStreamAdapter[Req, Resp]) Send(m *Resp) error { return a.Stream.SendMsg(m) }

// Receive reads one request message from the client.
func (a *BidiStreamAdapter[Req, Resp]) Receive() (*Req, error) {
	m := new(Req)
	if err := a.Stream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}
