// Handler context carrying transport metadata.
package runtime

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/peer"
)

// Ctx is the handler context. It carries the transport metadata (headers,
// query, body, cookies, path params), the negotiated codec, the transport
// protocol, the operation spec, the client peer, and the typed
// request/response payloads. Handlers receive a Ctx instead of a bare
// context.Context, so transport data and protocol detection are one method
// call away. Ctx satisfies context.Context (deadline, cancellation, and
// values delegate to the wrapped context).
type Ctx struct {
	ctx   context.Context
	state *ctxState
}

// ctxState holds the per-request data shared by all copies of a Ctx. The
// Ctx is passed by value through the unary interceptor chain; the state
// pointer lets WithValue/SetResponse on one copy be visible to the others.
type ctxState struct {
	meta     metadata.RequestMeta
	codec    encoding.Codec
	protocol string
	spec     Spec
	peer     peer.Peer
	req      any
	resp     any
}

// NewCtx wraps a context.Context with transport metadata and a codec. The
// metadata lives in the shared state (ctx.Meta()); the wrapped context is
// the plain request context so the hot path pays one allocation, not two.
// Code that needs the metadata from a bare context.Context (middleware,
// the metadata.*FromContext helpers) must attach it explicitly with
// metadata.WithRequestMeta.
func NewCtx(ctx context.Context, meta metadata.RequestMeta, codec encoding.Codec) Ctx {
	return Ctx{
		ctx:   ctx,
		state: &ctxState{meta: meta, codec: codec},
	}
}

// Deadline implements context.Context.
func (c Ctx) Deadline() (time.Time, bool) { return c.ctx.Deadline() }

// Done implements context.Context.
func (c Ctx) Done() <-chan struct{} { return c.ctx.Done() }

// Err implements context.Context.
func (c Ctx) Err() error { return c.ctx.Err() }

// Value implements context.Context.
func (c Ctx) Value(key any) any { return c.ctx.Value(key) }

// Context returns the wrapped context.Context. Use it when handing the
// request off to APIs that take a plain context (e.g. the gRPC gateway
// client calls); deadline, cancellation, and values propagate unchanged.
func (c Ctx) Context() context.Context {
	return c.ctx
}

// Meta returns the transport metadata (headers, query, body, cookies,
// path params).
func (c Ctx) Meta() metadata.RequestMeta {
	if c.state == nil {
		return metadata.RequestMeta{}
	}
	return c.state.meta
}

// Header returns the request headers.
func (c Ctx) Header() http.Header {
	if c.state == nil || c.state.meta.Header == nil {
		return make(http.Header)
	}
	return c.state.meta.Header
}

// HeaderGet returns the first value of the named request header.
func (c Ctx) HeaderGet(name string) string {
	return c.Header().Get(name)
}

// Query returns the URL query parameters.
func (c Ctx) Query() url.Values {
	if c.state == nil || c.state.meta.Query == nil {
		return make(url.Values)
	}
	return c.state.meta.Query
}

// QueryGet returns the first value of the named query parameter.
func (c Ctx) QueryGet(name string) string {
	return c.Query().Get(name)
}

// Body returns the raw request body bytes. It is populated for unary and
// server-streaming requests; streaming request bodies are not buffered.
func (c Ctx) Body() []byte {
	if c.state == nil {
		return nil
	}
	return c.state.meta.Body
}

// Cookies returns the request cookies.
func (c Ctx) Cookies() []*http.Cookie {
	if c.state == nil {
		return nil
	}
	return c.state.meta.Cookies
}

// Cookie returns the named cookie, or http.ErrNoCookie.
func (c Ctx) Cookie(name string) (*http.Cookie, error) {
	for _, cookie := range c.Cookies() {
		if cookie.Name == name {
			return cookie, nil
		}
	}
	return nil, http.ErrNoCookie
}

// Params returns the path parameters extracted from the route template,
// or nil when the route has no path parameters.
func (c Ctx) Params() map[string]string {
	if c.state == nil {
		return nil
	}
	return c.state.meta.Params
}

// Param returns the named path parameter.
func (c Ctx) Param(name string) (string, bool) {
	v, ok := c.Params()[name]
	return v, ok
}

// Codec returns the negotiated codec for the request.
func (c Ctx) Codec() encoding.Codec {
	if c.state == nil {
		return nil
	}
	return c.state.codec
}

// CodecName returns the negotiated codec name, or "" when no codec was
// negotiated.
func (c Ctx) CodecName() string {
	if codec := c.Codec(); codec != nil {
		return codec.Name()
	}
	return ""
}

// IsJsonCodec reports whether the request was encoded as JSON. HTTP/JSON
// requests negotiate the JSON codec; gRPC requests negotiate the protobuf
// codec, so this is the usual discriminator between the two transports.
func (c Ctx) IsJsonCodec() bool {
	return c.CodecName() == metadata.ContentTypeJSON
}

// IsProtobufCodec reports whether the request was encoded as protobuf.
func (c Ctx) IsProtobufCodec() bool {
	return c.CodecName() == metadata.ContentTypeProtobuf
}

// IsXMLCodec reports whether the request was encoded as XML.
func (c Ctx) IsXMLCodec() bool {
	return c.CodecName() == metadata.ContentTypeXML
}

// Protocol returns the transport protocol the request arrived over:
// "http", "grpc", "websocket", or "unknown".
func (c Ctx) Protocol() string {
	if c.state == nil || c.state.protocol == "" {
		return "unknown"
	}
	return c.state.protocol
}

// IsGRPC reports whether the request arrived over gRPC.
func (c Ctx) IsGRPC() bool {
	return c.Protocol() == "grpc"
}

// Spec returns the operation spec.
func (c Ctx) Spec() Spec {
	if c.state == nil {
		return Spec{}
	}
	return c.state.spec
}

// Peer returns the client peer info.
func (c Ctx) Peer() peer.Peer {
	if c.state == nil {
		return peer.Peer{}
	}
	return c.state.peer
}

// Request returns the typed request payload, or nil when the request has
// no single payload (client- and bidi-streaming) or the type does not
// match.
func (c Ctx) Request[T any]() *T {
	if c.state == nil {
		return nil
	}
	req, _ := c.state.req.(*T)
	return req
}

// Response returns the typed response payload, or nil when the response
// has not been set or the type does not match. The framework sets it after
// the handler returns; handlers and unary interceptors can set it earlier
// with SetResponse.
func (c Ctx) Response[T any]() *T {
	if c.state == nil {
		return nil
	}
	resp, _ := c.state.resp.(*T)
	return resp
}

// SetRequest stores the typed request payload on the Ctx. The framework
// calls it before the unary interceptor chain so every unary interceptor
// can read the payload generically.
func (c Ctx) SetRequest(req any) {
	if c.state != nil {
		c.state.req = req
	}
}

// SetResponse stores the typed response payload on the Ctx.
func (c Ctx) SetResponse(resp any) {
	if c.state != nil {
		c.state.resp = resp
	}
}

// WithValue returns a Ctx with an additional context value, preserving the
// shared request state. Interceptors use it instead of context.WithValue so
// the handler still receives a Ctx.
func (c Ctx) WithValue(key, value any) Ctx {
	return Ctx{
		ctx:   context.WithValue(c.ctx, key, value),
		state: c.state,
	}
}

// WithContext returns a Ctx wrapping a different context.Context,
// preserving the shared request state. It is used to propagate deadlines
// and cancellation derived from transport headers (Grpc-Timeout,
// Grpc-Deadline) onto the handler context.
func (c Ctx) WithContext(ctx context.Context) Ctx {
	return Ctx{
		ctx:   ctx,
		state: c.state,
	}
}

// WithCodec returns a Ctx with a different codec. It mutates the shared
// state; use it during construction (e.g. gRPC adapters marking protobuf
// requests).
func (c Ctx) WithCodec(codec encoding.Codec) Ctx {
	if c.state != nil {
		c.state.codec = codec
	}
	return c
}

// WithProtocol returns a Ctx with a different transport protocol. It
// mutates the shared state; use it during construction.
func (c Ctx) WithProtocol(protocol string) Ctx {
	if c.state != nil {
		c.state.protocol = protocol
	}
	return c
}

// WithSpec returns a Ctx with the operation spec. It mutates the shared
// state; use it during construction (e.g. gRPC adapters marking the RPC
// procedure so authorization policies can match on it).
func (c Ctx) WithSpec(spec Spec) Ctx {
	if c.state != nil {
		c.state.spec = spec
	}
	return c
}

// WithPeer returns a Ctx with the client peer info. It mutates the shared
// state; use it during construction (e.g. the HTTP handler marking the
// remote address and protocol).
func (c Ctx) WithPeer(peer peer.Peer) Ctx {
	if c.state != nil {
		c.state.peer = peer
	}
	return c
}
