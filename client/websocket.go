// WebSocket transport helpers.
package client

import (
	"context"
	"net/url"
	"reflect"
	"strings"

	"github.com/coder/websocket"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/status"
)

// wsServerStreamClient reads server-streaming responses over a WebSocket.
// The request message is sent as the first frame; each response arrives as
// one frame.
type wsServerStreamClient struct {
	conn   *websocket.Conn
	codec  encoding.Codec
	msgTyp reflect.Type
}

func (s *wsServerStreamClient) Receive() (any, error) {
	typ, data, err := s.conn.Read(context.Background())
	if err != nil {
		return nil, err
	}
	// Accept both text and binary frames; the codec decides what is valid.
	if typ != websocket.MessageText && typ != websocket.MessageBinary {
		return nil, status.NewErrorf(codes.CodeInvalidArgument, "unexpected websocket message type %d", typ)
	}
	msg := reflect.New(s.msgTyp).Interface()
	if err := s.codec.Unmarshal(data, msg); err != nil {
		return nil, status.NewErrorf(codes.CodeInternal, "decode stream message: %v", err)
	}
	return msg, nil
}

func (s *wsServerStreamClient) Close() error {
	return s.conn.Close(websocket.StatusNormalClosure, "")
}

// doServerStreamWS starts a server-streaming RPC over WebSocket. The
// request message is sent as the first frame; each response arrives as one
// frame.
func (c *ClientBase) doServerStreamWS(ctx context.Context, path string, req any, msgTyp reflect.Type, cfg clientConfig) (ServerStreamClient, error) {
	url := wsURL(c.baseURL, path, cfg.codec.Name())

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPClient: cfg.httpClient,
		HTTPHeader: cfg.headers,
	})
	if err != nil {
		return nil, err
	}

	data, err := cfg.codec.Marshal(req)
	if err != nil {
		conn.Close(websocket.StatusInternalError, "")
		return nil, status.NewErrorf(codes.CodeInternal, "encode request: %v", err)
	}
	typ := websocket.MessageText
	if cfg.codec.IsBinary() {
		typ = websocket.MessageBinary
	}
	if err := conn.Write(ctx, typ, data); err != nil {
		conn.Close(websocket.StatusInternalError, "")
		return nil, err
	}

	stream := &wsServerStreamClient{
		conn:   conn,
		codec:  cfg.codec,
		msgTyp: msgTyp,
	}
	if len(cfg.streamInterceptors) > 0 {
		wrapped := middleware.ChainStreamInterceptors(cfg.streamInterceptors, &serverStreamReceiver{ctx: ctx, codec: cfg.codec, inner: stream})
		return &interceptedServerStream{wrapped: wrapped, inner: stream}, nil
	}
	return stream, nil
}

// wsURL converts an http(s) base URL plus path into a ws(s) URL and
// appends the codec name as the "encoding" query parameter so the server
// can negotiate the codec without Content-Type/Accept headers.
func wsURL(baseURL, path, codecName string) string {
	u := baseURL + path
	if strings.HasPrefix(baseURL, "http://") {
		u = "ws://" + strings.TrimPrefix(baseURL, "http://") + path
	} else if strings.HasPrefix(baseURL, "https://") {
		u = "wss://" + strings.TrimPrefix(baseURL, "https://") + path
	}
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return u + sep + "encoding=" + url.QueryEscape(codecName)
}

// wsClientStream adapts a WebSocket connection to the client-side
// BidiStreamClient interface.
type wsClientStream struct {
	conn   *websocket.Conn
	codec  encoding.Codec
	reqTyp reflect.Type
}

func (s *wsClientStream) Send(msg any) error {
	data, err := s.codec.Marshal(msg)
	if err != nil {
		return status.NewErrorf(codes.CodeInternal, "encode request: %v", err)
	}
	typ := websocket.MessageText
	if s.codec.IsBinary() {
		typ = websocket.MessageBinary
	}
	return s.conn.Write(context.Background(), typ, data)
}

func (s *wsClientStream) Receive() (any, error) {
	typ, data, err := s.conn.Read(context.Background())
	if err != nil {
		return nil, err
	}
	// Accept both text and binary frames; the codec decides what is valid.
	if typ != websocket.MessageText && typ != websocket.MessageBinary {
		return nil, status.NewErrorf(codes.CodeInvalidArgument, "unexpected websocket message type %d", typ)
	}
	msg := reflect.New(s.reqTyp).Interface()
	if err := s.codec.Unmarshal(data, msg); err != nil {
		return nil, status.NewErrorf(codes.CodeInternal, "decode stream message: %v", err)
	}
	return msg, nil
}

func (s *wsClientStream) Close() error {
	return s.conn.Close(websocket.StatusNormalClosure, "")
}

// DialWebSocket connects to a bidi-streaming operation. It is used by
// generated clients.
func DialWebSocket(ctx context.Context, url string, opts ...ClientOption) (BidiStreamClient, error) {
	cfg := defaultClientConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPClient: cfg.httpClient,
		HTTPHeader: cfg.headers,
	})
	if err != nil {
		return nil, err
	}
	stream := &wsClientStream{
		conn:   conn,
		codec:  cfg.codec,
		reqTyp: cfg.reqType,
	}
	if len(cfg.streamInterceptors) > 0 {
		wrapped := middleware.ChainStreamInterceptors(cfg.streamInterceptors, &bidiStreamBoth{ctx: ctx, codec: cfg.codec, inner: stream})
		return &interceptedBidiStream{wrapped: wrapped, inner: stream}, nil
	}
	return stream, nil
}
