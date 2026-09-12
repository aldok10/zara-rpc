// Typed request wrapper.
package runtime

import (
	"net/http"
	"net/url"

	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/peer"
)

// Spec describes an RPC operation.
type Spec struct {
	// Procedure is the fully-qualified procedure name, e.g.
	// "/acme.users.v1.UsersService/GetUser".
	Procedure string
	// Method is the HTTP method (GET, POST, PUT, DELETE, PATCH).
	Method string
	// Path is the path template, e.g. "/v1/users/{id}".
	Path string
	// Service is the service name, e.g. "acme.users.v1.UsersService".
	Service string
	// RPC is the RPC method name, e.g. "GetUser".
	RPC string
}

// Request wraps a request message with metadata.
type Request[T any] struct {
	msg    *T
	header http.Header
	spec   Spec
	peer   peer.Peer
	meta   metadata.RequestMeta
}

// NewRequest creates a new Request wrapping msg.
func NewRequest[T any](msg *T) *Request[T] {
	return &Request[T]{
		msg:    msg,
		header: make(http.Header),
	}
}

// NewRequestWithMeta creates a Request with metadata. Used by generated
// request builders.
func NewRequestWithMeta[T any](msg *T, header http.Header, spec Spec, peer peer.Peer) *Request[T] {
	return &Request[T]{
		msg:    msg,
		header: header,
		spec:   spec,
		peer:   peer,
	}
}

// WithRequestMeta attaches transport metadata (headers, query, body,
// cookies) to the request. It is called by the operation machinery after
// the request is built.
func (r *Request[T]) WithRequestMeta(meta metadata.RequestMeta) *Request[T] {
	r.meta = meta
	return r
}

// Msg returns the wrapped message.
func (r *Request[T]) Msg() *T {
	return r.msg
}

// Header returns the mutable request headers.
func (r *Request[T]) Header() http.Header {
	return r.header
}

// Query returns the URL query parameters.
func (r *Request[T]) Query() url.Values {
	return r.meta.Query
}

// Body returns the raw request body bytes (unary and server-streaming
// requests only).
func (r *Request[T]) Body() []byte {
	return r.meta.Body
}

// Cookies returns the request cookies.
func (r *Request[T]) Cookies() []*http.Cookie {
	return r.meta.Cookies
}

// Cookie returns the named cookie, or http.ErrNoCookie.
func (r *Request[T]) Cookie(name string) (*http.Cookie, error) {
	for _, c := range r.meta.Cookies {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, http.ErrNoCookie
}

// Spec returns the operation spec.
func (r *Request[T]) Spec() Spec {
	return r.spec
}

// Peer returns the client peer info.
func (r *Request[T]) Peer() peer.Peer {
	return r.peer
}

// AnyRequest is the type-erased interface for request messages, used by
// unary interceptors.
type AnyRequest interface {
	Any() any
	Spec() Spec
	Peer() peer.Peer
	Header() http.Header
}

// rawRequest adapts a decoded message to AnyRequest for WebSocket
// transports where there is no HTTP request to build a typed Request from.
type rawRequest struct {
	msg    any
	spec   Spec
	peer   peer.Peer
	header http.Header
}

func (r *rawRequest) Any() any            { return r.msg }
func (r *rawRequest) Spec() Spec          { return r.spec }
func (r *rawRequest) Peer() peer.Peer     { return r.peer }
func (r *rawRequest) Header() http.Header { return r.header }

// Any returns the underlying message.
func (r *Request[T]) Any() any {
	return r.msg
}
