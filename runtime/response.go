// Typed response wrapper.
package runtime

import "net/http"

// Response wraps a response message with metadata.
type Response[T any] struct {
	msg     *T
	header  http.Header
	trailer http.Header
}

// NewResponse creates a new Response wrapping msg. The header and trailer
// maps are allocated lazily on first access.
func NewResponse[T any](msg *T) *Response[T] {
	return &Response[T]{msg: msg}
}

// Msg returns the wrapped message.
func (r *Response[T]) Msg() *T {
	return r.msg
}

// Header returns the mutable response headers.
func (r *Response[T]) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

// Trailer returns the mutable response trailers.
func (r *Response[T]) Trailer() http.Header {
	if r.trailer == nil {
		r.trailer = make(http.Header)
	}
	return r.trailer
}

// AnyResponse is the type-erased interface for response messages, used by
// unary interceptors and the HTTP handler writer.
type AnyResponse interface {
	Any() any
	Header() http.Header
	Trailer() http.Header
	HeaderSet() bool
}

// Any returns the underlying message.
func (r *Response[T]) Any() any {
	return r.msg
}

// HeaderSet reports whether the handler set response headers. The writer
// uses it to skip the lazy header map allocation when no headers were set.
func (r *Response[T]) HeaderSet() bool {
	return r.header != nil
}
