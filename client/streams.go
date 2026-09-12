// Streaming call helpers and raw stream interfaces.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"reflect"
	"sync"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/internal/xsync"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

// ---------------------------------------------------------------------------
// Raw client stream interfaces
// ---------------------------------------------------------------------------

// ServerStreamClient is the raw client-side view of a server-streaming
// RPC. Generated clients wrap it in ServerStreamForClient for type safety.
type ServerStreamClient interface {
	// Receive reads the next response message. It returns io.EOF when the
	// server has finished sending.
	Receive() (any, error)

	Close() error
}

// ClientStreamClient is the raw client-side view of a client-streaming
// RPC. Generated clients wrap it in ClientStreamForClient.
type ClientStreamClient interface {
	Send(any) error

	CloseAndReceive() (any, error)

	Close() error
}

// BidiStreamClient is the raw client-side view of a bidi-streaming RPC.
// Generated clients wrap it in BidiStreamForClient.
type BidiStreamClient interface {
	Send(any) error

	Receive() (any, error)

	Close() error
}

// ---------------------------------------------------------------------------
// Typed wrappers (constructed by generated clients)
// ---------------------------------------------------------------------------

// ServerStreamForClient is a typed wrapper around a raw server-streaming
// response.
type ServerStreamForClient[Resp any] struct {
	Inner ServerStreamClient
}

func (s *ServerStreamForClient[Resp]) Receive() (*Resp, error) {
	msg, err := s.Inner.Receive()
	if err != nil {
		return nil, err
	}
	typed, ok := msg.(*Resp)
	if !ok {
		return nil, status.NewErrorf(codes.CodeInternal, "unexpected message type %T", msg)
	}
	return typed, nil
}

func (s *ServerStreamForClient[Resp]) Close() error {
	return s.Inner.Close()
}

// ClientStreamForClient is a typed wrapper around a raw client-streaming
// request stream.
type ClientStreamForClient[Req, Resp any] struct {
	Inner ClientStreamClient
}

func (s *ClientStreamForClient[Req, Resp]) Send(msg *Req) error {
	return s.Inner.Send(msg)
}

func (s *ClientStreamForClient[Req, Resp]) CloseAndReceive() (*Resp, error) {
	raw, err := s.Inner.CloseAndReceive()
	if err != nil {
		return nil, err
	}
	typed, ok := raw.(*Resp)
	if !ok {
		return nil, status.NewErrorf(codes.CodeInternal, "unexpected response type %T", raw)
	}
	return typed, nil
}

func (s *ClientStreamForClient[Req, Resp]) Close() error {
	return s.Inner.Close()
}

// BidiStreamForClient is a typed wrapper around a raw bidi-streaming
// connection.
type BidiStreamForClient[Req, Resp any] struct {
	Inner BidiStreamClient
}

func (s *BidiStreamForClient[Req, Resp]) Send(msg *Req) error {
	return s.Inner.Send(msg)
}

func (s *BidiStreamForClient[Req, Resp]) Receive() (*Resp, error) {
	msg, err := s.Inner.Receive()
	if err != nil {
		return nil, err
	}
	typed, ok := msg.(*Resp)
	if !ok {
		return nil, status.NewErrorf(codes.CodeInternal, "unexpected message type %T", msg)
	}
	return typed, nil
}

func (s *BidiStreamForClient[Req, Resp]) Close() error {
	return s.Inner.Close()
}

// ---------------------------------------------------------------------------
// Stream transports
// ---------------------------------------------------------------------------

// sseDataPrefix is the SSE event data field marker. It is a package-level
// []byte so the hot Receive loop never converts the literal per event.
var sseDataPrefix = []byte("data:")

// Pooled buffers for framework-owned I/O. Safe to reuse because the data
// is decoded or copied out before the buffer returns to the pool (see the
// Codec contract: Unmarshal must not retain its input, Marshal returns a
// fresh slice). Buffers larger than 1 MiB are dropped, not retained.
var (
	// clientReadBufPool holds unary response bodies (read -> Unmarshal).
	clientReadBufPool = xsync.NewBuffer(1 << 20)
	// sseReadBufPool holds base64 decode buffers for binary SSE events.
	sseReadBufPool = xsync.NewBytes(1 << 20)
)

// interceptedServerStream wraps a receive-only stream with stream
// interceptors.
type interceptedServerStream struct {
	wrapped middleware.Stream
	inner   ServerStreamClient
}

func (s *interceptedServerStream) Context() runtime.Ctx  { return s.wrapped.Context() }
func (s *interceptedServerStream) Receive() (any, error) { return s.wrapped.Receive() }
func (s *interceptedServerStream) Close() error          { return s.inner.Close() }

// interceptedClientStream wraps a send-only stream with stream
// interceptors.
type interceptedClientStream struct {
	wrapped middleware.Stream
	inner   ClientStreamClient
}

func (s *interceptedClientStream) Context() runtime.Ctx          { return s.wrapped.Context() }
func (s *interceptedClientStream) Send(msg any) error            { return s.wrapped.Send(msg) }
func (s *interceptedClientStream) CloseAndReceive() (any, error) { return s.inner.CloseAndReceive() }
func (s *interceptedClientStream) Close() error                  { return s.inner.Close() }

// interceptedBidiStream wraps a bidirectional stream with stream
// interceptors.
type interceptedBidiStream struct {
	wrapped middleware.Stream
	inner   BidiStreamClient
}

func (s *interceptedBidiStream) Context() runtime.Ctx  { return s.wrapped.Context() }
func (s *interceptedBidiStream) Send(msg any) error    { return s.wrapped.Send(msg) }
func (s *interceptedBidiStream) Receive() (any, error) { return s.wrapped.Receive() }
func (s *interceptedBidiStream) Close() error          { return s.inner.Close() }

// serverStreamReceiver adapts a receive-only ServerStreamClient to
// runtime.Stream for stream interceptor wrapping. The unused Send direction
// returns an error.
type serverStreamReceiver struct {
	ctx   context.Context
	codec encoding.Codec
	inner ServerStreamClient
}

func (s *serverStreamReceiver) Context() runtime.Ctx {
	return runtime.NewCtx(s.ctx, metadata.RequestMeta{}, s.codec)
}
func (s *serverStreamReceiver) Send(any) error {
	return status.NewErrorf(codes.CodeInternal, "send not supported on server-streaming transport")
}
func (s *serverStreamReceiver) Receive() (any, error) { return s.inner.Receive() }

// clientStreamSender adapts a send-only ClientStreamClient to
// runtime.Stream. The unused Receive direction returns an error.
type clientStreamSender struct {
	ctx   context.Context
	codec encoding.Codec
	inner ClientStreamClient
}

func (s *clientStreamSender) Context() runtime.Ctx {
	return runtime.NewCtx(s.ctx, metadata.RequestMeta{}, s.codec)
}
func (s *clientStreamSender) Send(msg any) error { return s.inner.Send(msg) }
func (s *clientStreamSender) Receive() (any, error) {
	return nil, status.NewErrorf(codes.CodeInternal, "receive not supported on client-streaming transport")
}

// bidiStreamBoth wraps a bidirectional BidiStreamClient that already
// satisfies runtime.Stream.
type bidiStreamBoth struct {
	ctx   context.Context
	codec encoding.Codec
	inner BidiStreamClient
}

func (s *bidiStreamBoth) Context() runtime.Ctx {
	return runtime.NewCtx(s.ctx, metadata.RequestMeta{}, s.codec)
}
func (s *bidiStreamBoth) Send(msg any) error    { return s.inner.Send(msg) }
func (s *bidiStreamBoth) Receive() (any, error) { return s.inner.Receive() }

// sseClientStream reads SSE events from a server-streaming response.
type sseClientStream struct {
	r      *bufio.Reader
	codec  encoding.Codec
	msgTyp reflect.Type
	body   io.ReadCloser
	done   chan struct{}
	once   sync.Once
}

func (s *sseClientStream) Receive() (any, error) {
	for {
		line, err := s.r.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, sseDataPrefix) {
			continue
		}
		data := bytes.TrimSpace(line[len(sseDataPrefix):])
		if len(data) == 0 {
			continue
		}
		// Binary codecs (e.g. protobuf) are base64-encoded inside the SSE
		// text framing. Decode straight from the bytes: converting to a
		// string first would copy the data. The decode buffer is pooled
		// (*[]byte, so Get/Put are allocation-free); it is framework-owned
		// and dead after Unmarshal.
		var pooled *[]byte
		if s.codec.IsBinary() {
			decoded := sseReadBufPool.GetN(base64.StdEncoding.DecodedLen(len(data)))
			n, err := base64.StdEncoding.Decode(*decoded, data)
			if err != nil {
				sseReadBufPool.Put(decoded)
				return nil, status.NewErrorf(codes.CodeInternal, "decode base64 stream message: %v", err)
			}
			data = (*decoded)[:n]
			pooled = decoded
		}
		msg := reflect.New(s.msgTyp).Interface()
		if err := s.codec.Unmarshal(data, msg); err != nil {
			if pooled != nil {
				sseReadBufPool.Put(pooled)
			}
			return nil, status.NewErrorf(codes.CodeInternal, "decode stream message: %v", err)
		}
		if pooled != nil {
			sseReadBufPool.Put(pooled)
		}
		return msg, nil
	}
}

func (s *sseClientStream) Close() error {
	s.once.Do(func() { close(s.done) })
	return s.body.Close()
}

// DoServerStream starts a server-streaming RPC and returns a stream of
// responses. Path parameters in the path template are substituted from the
// request message. When ctx is canceled, the underlying connection is
// closed so blocked Receive calls fail.
func (c *ClientBase) DoServerStream(ctx context.Context, method, path string, req any, msgTyp reflect.Type, opts ...ClientOption) (ServerStreamClient, error) {
	cfg := c.cfg
	cfg.headers = c.cfg.headers.Clone()
	for _, opt := range opts {
		opt(&cfg)
	}
	path, err := runtime.SubstitutePathParams(path, req)
	if err != nil {
		return nil, err
	}

	if cfg.serverStreamTransport == ServerStreamWebSocket {
		return c.doServerStreamWS(ctx, path, req, msgTyp, cfg)
	}

	var body io.Reader
	if req != nil {
		data, err := cfg.codec.Marshal(req)
		if err != nil {
			return nil, status.NewErrorf(codes.CodeInternal, "encode request: %v", err)
		}
		body = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	for k, vs := range cfg.headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	if req != nil {
		httpReq.Header.Set(metadata.HeaderContentType, cfg.codec.Name())
	}
	httpReq.Header.Set(metadata.HeaderAccept, metadata.ContentTypeEventStream)

	httpResp, err := cfg.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		return nil, status.FromHTTP(httpResp)
	}

	stream := &sseClientStream{
		r:      bufio.NewReader(httpResp.Body),
		codec:  cfg.codec,
		msgTyp: msgTyp,
		body:   httpResp.Body,
		done:   make(chan struct{}),
	}
	// Close the body when the context is canceled OR the stream is closed,
	// so blocked reads fail. Selecting on both prevents a goroutine leak
	// when the caller closes the stream without canceling the context.
	go func() {
		select {
		case <-ctx.Done():
		case <-stream.done:
		}
		httpResp.Body.Close()
	}()
	if len(cfg.streamInterceptors) > 0 {
		wrapped := middleware.ChainStreamInterceptors(cfg.streamInterceptors, &serverStreamReceiver{ctx: ctx, codec: cfg.codec, inner: stream})
		return &interceptedServerStream{wrapped: wrapped, inner: stream}, nil
	}
	return stream, nil
}

// DoClientStream starts a client-streaming RPC and returns a stream to
// write requests on. The HTTP request runs in a background goroutine
// because the server reads the request body until EOF before responding;
// http.Client.Do cannot complete until the client closes the stream.
func (c *ClientBase) DoClientStream(ctx context.Context, method, path string, respTyp reflect.Type, opts ...ClientOption) (ClientStreamClient, error) {
	cfg := c.cfg
	cfg.headers = c.cfg.headers.Clone()
	for _, opt := range opts {
		opt(&cfg)
	}
	pr, pw := io.Pipe()

	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, pr)
	if err != nil {
		return nil, err
	}
	for k, vs := range cfg.headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	httpReq.Header.Set(metadata.HeaderContentType, cfg.codec.Name())
	httpReq.Header.Set(metadata.HeaderAccept, cfg.codec.Name())

	respCh := make(chan ndjsonResult, 1)
	go func() {
		resp, err := cfg.httpClient.Do(httpReq)
		respCh <- ndjsonResult{resp: resp, err: err}
	}()

	stream := &ndjsonWriteStream{
		w:       pw,
		codec:   cfg.codec,
		body:    pw,
		respCh:  respCh,
		respTyp: respTyp,
	}
	if len(cfg.streamInterceptors) > 0 {
		wrapped := middleware.ChainStreamInterceptors(cfg.streamInterceptors, &clientStreamSender{ctx: ctx, codec: cfg.codec, inner: stream})
		return &interceptedClientStream{wrapped: wrapped, inner: stream}, nil
	}
	return stream, nil
}

// DoBidiStream connects to a bidi-streaming operation over WebSocket.
func (c *ClientBase) DoBidiStream(ctx context.Context, path string, reqTyp reflect.Type, opts ...ClientOption) (BidiStreamClient, error) {
	cfg := c.cfg
	cfg.headers = c.cfg.headers.Clone()
	for _, opt := range opts {
		opt(&cfg)
	}
	url := wsURL(c.baseURL, path, cfg.codec.Name())

	cfg.reqType = reqTyp
	stream, err := DialWebSocket(ctx, url, func(cc *clientConfig) {
		*cc = cfg
	})
	if err != nil {
		return nil, err
	}
	return stream, nil
}
