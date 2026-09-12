package kernel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/peer"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
	"github.com/aldok10/zara-rpc/streaming"
	"github.com/aldok10/zara-rpc/transport/ndjson"
	"github.com/aldok10/zara-rpc/transport/sse"
	"github.com/aldok10/zara-rpc/transport/websocket"
)

// Handler serves a single RPC operation over HTTP. It implements the
// request population, the unary interceptor chain, and response encoding.
// Path matching is owned by routing.Mux, which calls ServeHTTPWithParams
// with the params from its own route lookup; the Handler does not parse
// patterns itself.
type Handler struct {
	operation *Operation
	spec      runtime.Spec
	unary     middleware.UnaryFunc
	// streamAuth runs the unary interceptor chain without a handler, so
	// streaming operations are protected by the same auth chain as unary
	// operations. It is nil when the operation has no unary interceptors.
	streamAuth middleware.UnaryFunc
	// streamInterceptors wrap the stream object per message. They run
	// after streamAuth has authorized the request.
	streamInterceptors []middleware.StreamInterceptor
}

// NewHandler builds a Handler for an operation.
func NewHandler(e *Operation) *Handler {
	h := &Handler{
		operation:          e,
		spec:               e.spec(),
		streamInterceptors: e.streamInterceptors,
	}
	h.unary = middleware.ChainUnaryInterceptors(e.interceptors, e.unary)
	if len(e.interceptors) > 0 {
		h.streamAuth = middleware.ChainUnaryInterceptors(e.interceptors, func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
			return nil, nil
		})
	}
	return h
}

// wrapStream applies the stream interceptor chain to a stream, adapting
// unidirectional transports to the type-erased middleware.Stream view. It
// returns the stream unchanged when no stream interceptors are registered.
func (h *Handler) wrapStream(s middleware.Stream) middleware.Stream {
	if len(h.streamInterceptors) == 0 {
		return s
	}
	return middleware.ChainStreamInterceptors(h.streamInterceptors, s)
}

// Spec returns the operation spec.
// RPC returns the operation's RPC method name (e.g. "GetUser").
func (h *Handler) RPC() string { return h.operation.RPC }

func (h *Handler) Spec() runtime.Spec {
	return h.spec
}

// ServeHTTPWithParams handles a request whose path has already been
// matched. The mux calls this with the params from its own route lookup,
// avoiding a second Match (and its map allocation) per request.
func (h *Handler) ServeHTTPWithParams(w http.ResponseWriter, r *http.Request, params map[string]string) {
	// Parse the query string once; it feeds both codec negotiation (the
	// "encoding" parameter used by WebSocket transports) and the request
	// metadata.
	var query url.Values
	if r.URL.RawQuery != "" {
		query = r.URL.Query()
	}
	codec := codecFromRequest(r, query)

	// Derive the request context deadline from the gRPC deadline headers.
	// Grpc-Timeout wins over Grpc-Deadline (the gRPC convention); malformed
	// values are ignored (no deadline). The deadline is applied to the
	// wrapped context so ctx.Done()/ctx.Err() and the gateway's upstream
	// gRPC calls all observe it.
	reqCtx := r.Context()
	if d, ok := deadlineFromRequest(r); ok {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(reqCtx, d)
		defer cancel()
	}

	// Capture transport metadata (headers, query, cookies) for handlers.
	// The header map is the live request header (not a copy), matching
	// grpc-gateway: the request is per-request, so handler mutation cannot
	// leak across requests. The query is only parsed when present (body-only
	// requests keep a nil map, which range/Get handle safely). The raw body
	// is buffered only for single-message requests; streaming request bodies
	// stay streams.
	meta := metadata.RequestMeta{
		Header:  r.Header,
		Query:   query,
		Cookies: r.Cookies(),
		Params:  params,
	}
	if h.operation.streamType == runtime.StreamTypeUnary || h.operation.streamType == runtime.StreamTypeServer {
		if r.Body != nil && r.Body != http.NoBody {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, status.NewErrorf(codes.CodeInvalidArgument, "read body: %v", err))
				return
			}
			meta.Body = data
		}
	}
	// Build the handler context once. It carries the transport metadata,
	// the negotiated codec, the operation spec, and the client peer; the
	// typed request payload is attached after the request is built.
	ctx := runtime.NewCtx(reqCtx, meta, codec)
	ctx = ctx.WithProtocol("http").WithSpec(h.spec).WithPeer(peer.Peer{Addr: r.RemoteAddr, Protocol: r.Proto})
	if isWebSocketUpgrade(r) {
		ctx = ctx.WithProtocol("websocket")
	}

	switch h.operation.streamType {
	case runtime.StreamTypeServer:
		h.serveServerStream(w, r, ctx, params, codec)
	case runtime.StreamTypeClient:
		h.serveClientStream(w, r, ctx, params, codec)
	case runtime.StreamTypeBidi:
		h.serveBidiStream(w, r, ctx, params, codec)
	default:
		h.serveUnary(w, r, ctx, params, codec)
	}
}

func (h *Handler) serveUnary(w http.ResponseWriter, r *http.Request, ctx runtime.Ctx, params map[string]string, codec encoding.Codec) {
	req, err := h.operation.newRequest(ctx, r, params, codec)
	if err != nil {
		writeError(w, err)
		return
	}
	// Attach the typed payload before the unary interceptor chain so every
	// unary interceptor can read it generically via ctx.Request[T]().
	ctx.SetRequest(req.Any())

	resp, err := h.unary(ctx, req)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx.SetResponse(resp.Any())

	h.writeResponse(w, r, resp, codec)
}

// serveServerStream handles a server-streaming RPC over SSE, or over
// WebSocket when the client requests an upgrade.
func (h *Handler) serveServerStream(w http.ResponseWriter, r *http.Request, ctx runtime.Ctx, params map[string]string, codec encoding.Codec) {
	if err := h.authorizeStream(w, ctx); err != nil {
		return
	}
	req, err := h.operation.newRequest(ctx, r, params, codec)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx.SetRequest(req.Any())

	// WebSocket upgrade requests are served over WebSocket frames;
	// everything else uses SSE.
	if isWebSocketUpgrade(r) {
		h.serveServerStreamWS(w, r, ctx, codec)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, status.NewErrorf(codes.CodeInternal, "streaming unsupported by this server"))
		return
	}

	w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeEventStream)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set(metadata.HeaderConnection, metadata.ConnectionKeepAlive)
	w.WriteHeader(http.StatusOK)
	// Flush the headers immediately so the client's request completes and
	// the stream is established before the handler produces its first event.
	flusher.Flush()

	stream := h.wrapStream(streaming.NewServerStreamAdapter(ctx, sse.NewServerStream(w, flusher, codec)))
	if err := h.operation.serverStream(stream.Context(), req, stream); err != nil {
		// The stream may already be partially written; an error after
		// headers are sent cannot change the status code.
		return
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get(metadata.HeaderUpgrade), metadata.UpgradeWebSocket) &&
		headerContainsFold(r.Header.Get(metadata.HeaderConnection), metadata.ConnectionUpgrade)
}

// headerContainsFold reports whether the comma-separated header value
// contains the token, case-insensitively, without allocating (no ToLower
// copy, no Split slice).
func headerContainsFold(v, token string) bool {
	for len(v) > 0 {
		i := strings.IndexByte(v, ',')
		var part string
		if i < 0 {
			part = v
			v = ""
		} else {
			part = v[:i]
			v = v[i+1:]
		}
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// codecFromRequest negotiates the codec for a request. WebSocket dials
// carry no Content-Type or Accept header, so clients pass the codec name
// explicitly as the "encoding" query parameter; it wins when present.
// query is the already-parsed query string (nil when the request had no
// query).
func codecFromRequest(r *http.Request, query url.Values) encoding.Codec {
	if query != nil {
		if enc := query.Get("encoding"); enc != "" {
			return encoding.CodecForContentType(enc)
		}
	}
	// Inline the CodecForRequest fast path: the common case (no
	// Content-Type, no Accept, or Accept is "*/*") returns JSON. Direct
	// map lookups skip Header.Get's method overhead and the CodecForRequest
	// function call for this path.
	if _, ok := r.Header[metadata.HeaderContentType]; ok {
		return encoding.CodecForContentType(r.Header.Get(metadata.HeaderContentType))
	}
	if accept := r.Header.Get(metadata.HeaderAccept); accept != "" && accept != "*/*" {
		return encoding.CodecForContentType(accept)
	}
	return encoding.JSONCodec{}
}

// deadlineFromRequest derives the request deadline from the gRPC deadline
// headers. Grpc-Timeout wins over Grpc-Deadline (the gRPC convention);
// malformed values are ignored (ok=false, no deadline). The returned
// duration is relative to now.
func deadlineFromRequest(r *http.Request) (time.Duration, bool) {
	if v := r.Header.Get(metadata.HeaderGrpcTimeout); v != "" {
		if d, ok := metadata.ParseGrpcTimeout(v); ok {
			return d, true
		}
	}
	if v := r.Header.Get(metadata.HeaderGrpcDeadline); v != "" {
		if t, ok := metadata.ParseGrpcDeadline(v); ok {
			if d := time.Until(t); d > 0 {
				return d, true
			}
			// The deadline is already in the past; use a zero duration so
			// the context is immediately expired.
			return 0, true
		}
	}
	return 0, false
}

// serveServerStreamWS handles a server-streaming RPC over WebSocket. The
// request message arrives as the first frame; each response is written as
// one frame.
func (h *Handler) serveServerStreamWS(w http.ResponseWriter, r *http.Request, ctx runtime.Ctx, codec encoding.Codec) {
	conn, err := websocket.Accept(w, r)
	if err != nil {
		writeError(w, status.NewErrorf(codes.CodeInternal, "websocket upgrade: %v", err))
		return
	}

	// The request message is the first frame.
	data, err := conn.Read(ctx)
	if err != nil {
		conn.Close(1011, "read request: "+err.Error())
		return
	}
	req, err := h.operation.decodeRequest(data, codec)
	if err != nil {
		conn.Close(1011, err.Error())
		return
	}

	stream := h.wrapStream(streaming.NewServerStreamAdapter(ctx, websocket.NewServerStream(conn, codec)))
	if err := h.operation.serverStream(stream.Context(), req, stream); err != nil {
		conn.Close(1011, err.Error())
		return
	}

	conn.Close(1000, "")
}

// serveClientStream handles a client-streaming RPC over NDJSON.
func (h *Handler) serveClientStream(w http.ResponseWriter, r *http.Request, ctx runtime.Ctx, params map[string]string, codec encoding.Codec) {
	if err := h.authorizeStream(w, ctx); err != nil {
		return
	}
	stream := h.wrapStream(streaming.NewClientStreamAdapter(ctx, ndjson.NewClientStream(r.Body, codec, h.operation.reqType)))

	resp, err := h.operation.clientStream(stream.Context(), stream)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx.SetResponse(resp.Any())

	h.writeResponse(w, r, resp, codec)
}

// serveBidiStream handles a bidi-streaming RPC over WebSocket.
func (h *Handler) serveBidiStream(w http.ResponseWriter, r *http.Request, ctx runtime.Ctx, params map[string]string, codec encoding.Codec) {
	if err := h.authorizeStream(w, ctx); err != nil {
		return
	}
	conn, err := websocket.Accept(w, r)
	if err != nil {
		writeError(w, status.NewErrorf(codes.CodeInternal, "websocket upgrade: %v", err))
		return
	}

	stream := h.wrapStream(streaming.NewBidiStreamAdapter(ctx, websocket.NewBidiStream(conn, codec, h.operation.reqType, h.operation.resType)))
	if err := h.operation.bidiStream(stream.Context(), stream); err != nil {
		conn.Close(1011, err.Error())
		return
	}

	conn.Close(1000, "")
}

// authorizeStream runs the unary interceptor chain on a streaming request
// before the stream is established. It writes the error response and
// returns true when the request was rejected. Operations without unary
// interceptors skip the check entirely (streamAuth is nil).
func (h *Handler) authorizeStream(w http.ResponseWriter, ctx runtime.Ctx) error {
	if h.streamAuth == nil {
		return nil
	}
	if _, err := h.streamAuth(ctx, nil); err != nil {
		writeError(w, err)
		return err
	}
	return nil
}

func (h *Handler) writeResponse(w http.ResponseWriter, r *http.Request, resp runtime.AnyResponse, codec encoding.Codec) {
	// Copy response headers. The lazy map is only allocated when the
	// handler actually set headers.
	if resp.HeaderSet() {
		for k, vs := range resp.Header() {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
	}
	w.Header().Set(metadata.HeaderContentType, codec.Name())

	// Select the response body field if configured.
	body := resp.Any()
	if h.operation.ResponseBody != "" {
		selected, err := extractField(resp.Any(), h.operation.ResponseBody)
		if err != nil {
			writeError(w, err)
			return
		}
		body = selected
	}

	data, err := codec.Marshal(body)
	if err != nil {
		writeError(w, status.NewErrorf(codes.CodeInternal, "encode response: %v", err))
		return
	}
	// The response bytes are NOT pooled: returning them to a sync.Pool would
	// box the []byte into an interface (one allocation per request) and
	// regress the mux hot-path budget. Marshal still allocates; that is the
	// documented price of correct proto encoding.
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		// The client is gone; nothing more to do.
		return
	}
}

// writeError writes an RPC error as an HTTP response with a JSON body.
func writeError(w http.ResponseWriter, err error) {
	code := status.Code(err)
	statusCode := code.HTTPStatus()

	var rpcErr *status.Error
	if !errors.As(err, &rpcErr) {
		rpcErr = status.NewError(code, err)
	}

	// Copy error metadata headers.
	for k, vs := range rpcErr.Meta() {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)

	// grpc-status-details-bin carries the base64 google.rpc.Status so
	// gRPC-aware clients (and the gateway) recover the full detail payloads
	// instead of only the JSON code/message.
	if details := rpcErr.Details(); len(details) > 0 {
		if data, err := rpcErr.MarshalStatus(); err == nil {
			w.Header().Set(metadata.HeaderGrpcStatusDetails, base64.StdEncoding.EncodeToString(data))
		}
	}

	body := map[string]any{
		"code":    code.String(),
		"message": rpcErr.Message(),
	}
	data, _ := json.Marshal(body)

	w.WriteHeader(statusCode)
	_, _ = w.Write(data)
}

// extractField returns a named field of a message, supporting dot notation.
func extractField(msg any, name string) (any, error) {
	v := reflect.ValueOf(msg)
	parts := strings.Split(name, ".")
	for _, part := range parts {
		f, ok := runtime.FindField(v, part)
		if !ok {
			return nil, status.NewErrorf(codes.CodeInternal, "response field %q not found in %T", name, msg)
		}
		v = f
	}
	return v.Interface(), nil
}
