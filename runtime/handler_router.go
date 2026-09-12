// handler_router.go: protocol dispatch on one listener.
//
// A zararpc server that shares one h2c listener between gRPC and HTTP needs
// two-way dispatch: gRPC requests (content-type: application/grpc over
// HTTP/2) go to the grpc-go server, everything else goes to the zararpc mux.
// HandlerRouter owns that dispatch so applications wire handlers instead of
// hand-rolling an http.HandlerFunc. Path-level filtering (public vs
// protected) is an interceptor concern, not a router concern: the mux runs
// one interceptor chain, and the chain decides which requests skip auth.
package runtime

import (
	"net/http"
	"strings"

	"github.com/aldok10/zara-rpc/metadata"
)

// HandlerRouter routes requests between the gRPC server and the zararpc mux
// on one listener. The mux serves every HTTP request; interceptors on the
// mux decide path-level policy (e.g. public auth endpoints).
type HandlerRouter struct {
	grpc http.Handler
	mux  http.Handler
}

// HandlerRouterOption configures a HandlerRouter.
type HandlerRouterOption func(*HandlerRouter)

// WithGRPC routes HTTP/2 requests whose content-type is application/grpc to
// h. The gRPC server is passed as a plain http.Handler, so the framework
// root stays grpc-free.
func WithGRPC(h http.Handler) HandlerRouterOption {
	return func(r *HandlerRouter) {
		r.grpc = h
	}
}

// NewHandlerRouter returns a router that sends every request not matched by
// WithGRPC to mux.
func NewHandlerRouter(mux http.Handler, opts ...HandlerRouterOption) *HandlerRouter {
	r := &HandlerRouter{mux: mux}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ServeHTTP dispatches in order: gRPC by content-type, then the mux.
func (r *HandlerRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if r.grpc != nil && req.ProtoMajor == 2 && strings.HasPrefix(req.Header.Get(metadata.HeaderContentType), metadata.ContentTypeGRPC) {
		r.grpc.ServeHTTP(w, req)
		return
	}
	r.mux.ServeHTTP(w, req)
}