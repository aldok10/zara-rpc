package routing

import (
	"net/http"
	"strings"

	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/middleware"
)

// ServerRegistrar is implemented by routing.Server, routing.Mux, and gRPC registrars.
type ServerRegistrar interface {
	Mux() *Mux
	GRPC() any
}

// DefaultGRPCFactory is initialized by the grpcbridge package so NewServer
// automatically enables gRPC support by default without extra options.
var DefaultGRPCFactory func(unary []middleware.UnaryInterceptor, stream []middleware.StreamInterceptor) (http.Handler, any)

// Server combines the zara-rpc HTTP Mux and an optional gRPC server into a
// single http.Handler with unified interceptor configuration.
type Server struct {
	mux           *Mux
	grpcHandler   http.Handler
	grpcRegistrar any
	grpcInit      func(unary []middleware.UnaryInterceptor, stream []middleware.StreamInterceptor) (http.Handler, any)
	disableGRPC   bool
	unaryChains   []middleware.UnaryInterceptor
	streamChains  []middleware.StreamInterceptor
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithUnaryInterceptors sets the unary interceptors applied to both HTTP
// operations and gRPC methods.
func WithUnaryInterceptors(interceptors ...middleware.UnaryInterceptor) ServerOption {
	return func(s *Server) {
		s.unaryChains = append(s.unaryChains, interceptors...)
	}
}

// WithStreamInterceptors sets the stream interceptors applied to both HTTP
// streams and gRPC streams.
func WithStreamInterceptors(interceptors ...middleware.StreamInterceptor) ServerOption {
	return func(s *Server) {
		s.streamChains = append(s.streamChains, interceptors...)
	}
}

// WithDisableGRPC disables default gRPC server initialization.
func WithDisableGRPC() ServerOption {
	return func(s *Server) {
		s.disableGRPC = true
	}
}

// WithGRPCHandler attaches a gRPC server handler and registrar to the Server.
func WithGRPCHandler(h http.Handler, reg any) ServerOption {
	return func(s *Server) {
		s.grpcHandler = h
		s.grpcRegistrar = reg
	}
}

// WithGRPCFactory attaches a lazy gRPC server factory that receives the
// unified interceptors configured on the Server.
func WithGRPCFactory(factory func(unary []middleware.UnaryInterceptor, stream []middleware.StreamInterceptor) (http.Handler, any)) ServerOption {
	return func(s *Server) {
		s.grpcInit = factory
	}
}

// NewServer creates a unified Server. By default, gRPC support with reflection
// and unified interceptors is enabled unless WithDisableGRPC is passed.
func NewServer(opts ...ServerOption) *Server {
	s := &Server{}
	for _, opt := range opts {
		opt(s)
	}
	s.mux = NewMux(
		WithMuxUnaryInterceptors(s.unaryChains...),
		WithMuxStreamInterceptors(s.streamChains...),
	)
	if !s.disableGRPC && s.grpcHandler == nil {
		if s.grpcInit == nil {
			s.grpcInit = DefaultGRPCFactory
		}
		if s.grpcInit != nil {
			s.grpcHandler, s.grpcRegistrar = s.grpcInit(s.unaryChains, s.streamChains)
		}
	}
	return s
}

// Mux returns the underlying Mux.
func (s *Server) Mux() *Mux {
	return s.mux
}

// GRPC returns the gRPC registrar if gRPC is enabled.
func (s *Server) GRPC() any {
	return s.grpcRegistrar
}

// UnaryInterceptors returns the configured unary interceptors.
func (s *Server) UnaryInterceptors() []middleware.UnaryInterceptor {
	return s.unaryChains
}

// StreamInterceptors returns the configured stream interceptors.
func (s *Server) StreamInterceptors() []middleware.StreamInterceptor {
	return s.streamChains
}

// ServeHTTP dispatches gRPC requests to the gRPC server and all other
// requests to the zararpc mux.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.grpcHandler != nil && r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get(metadata.HeaderContentType), metadata.ContentTypeGRPC) {
		s.grpcHandler.ServeHTTP(w, r)
		return
	}
	s.mux.ServeHTTP(w, r)
}