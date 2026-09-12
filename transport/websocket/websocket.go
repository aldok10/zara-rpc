// Package websocket implements the WebSocket streaming transports
// (server-streaming and bidi).
package websocket

import (
	"context"
	"net/http"
	"reflect"
	"sync"

	"github.com/coder/websocket"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/status"
)

// Accept upgrades an HTTP request to a WebSocket connection and wraps it
// in the Conn interface used by the streaming transports.
func Accept(w http.ResponseWriter, r *http.Request) (Conn, error) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, err
	}
	return &wsAdapter{conn: conn}, nil
}

// wsAdapter adapts *websocket.Conn to the Conn interface.
type wsAdapter struct {
	conn *websocket.Conn
}

func (a *wsAdapter) Read(ctx context.Context) ([]byte, error) {
	typ, data, err := a.conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	// Accept both text and binary frames; the codec decides what is valid.
	if typ != websocket.MessageText && typ != websocket.MessageBinary {
		return nil, status.NewErrorf(codes.CodeInvalidArgument, "unexpected websocket message type %d", typ)
	}
	return data, nil
}

func (a *wsAdapter) Write(ctx context.Context, typ int, data []byte) error {
	return a.conn.Write(ctx, websocket.MessageType(typ), data)
}

func (a *wsAdapter) Close(code int, reason string) error {
	return a.conn.Close(websocket.StatusCode(code), reason)
}

// ServerStream writes typed messages as WebSocket frames. It implements
// runtime.ServerStream[any] and is used by server-streaming operations
// when the client requests a WebSocket upgrade instead of SSE.
type ServerStream struct {
	conn  Conn
	codec encoding.Codec
	mu    sync.Mutex
}

// NewServerStream builds a WebSocket server stream over the connection.
func NewServerStream(conn Conn, codec encoding.Codec) *ServerStream {
	return &ServerStream{conn: conn, codec: codec}
}

// Send writes one message as a WebSocket frame.
func (s *ServerStream) Send(msg any) error {
	data, err := s.codec.Marshal(msg)
	if err != nil {
		return status.NewErrorf(codes.CodeInternal, "encode stream message: %v", err)
	}
	// Binary codecs use binary frames; text codecs use text frames.
	typ := 1 // websocket.MessageText
	if s.codec.IsBinary() {
		typ = 2 // websocket.MessageBinary
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.Write(context.Background(), typ, data)
}

// BidiStream reads and writes messages over a WebSocket connection. It
// implements runtime.BidiStream[any, any] and is used by bidi-streaming
// operations.
type BidiStream struct {
	conn    Conn
	codec   encoding.Codec
	reqTyp  reflect.Type
	respTyp reflect.Type
	readMu  sync.Mutex
	writeMu sync.Mutex
}

// NewBidiStream builds a WebSocket bidi stream over the connection.
func NewBidiStream(conn Conn, codec encoding.Codec, reqTyp, respTyp reflect.Type) *BidiStream {
	return &BidiStream{conn: conn, codec: codec, reqTyp: reqTyp, respTyp: respTyp}
}

// Send writes one response message as a WebSocket frame.
func (s *BidiStream) Send(msg any) error {
	data, err := s.codec.Marshal(msg)
	if err != nil {
		return status.NewErrorf(codes.CodeInternal, "encode stream message: %v", err)
	}
	// Binary codecs use binary frames; text codecs use text frames.
	typ := 1 // websocket.MessageText
	if s.codec.IsBinary() {
		typ = 2 // websocket.MessageBinary
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.Write(context.Background(), typ, data)
}

// Receive reads the next request message.
func (s *BidiStream) Receive() (any, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	data, err := s.conn.Read(context.Background())
	if err != nil {
		return nil, err
	}
	msg := reflect.New(s.reqTyp).Interface()
	if err := s.codec.Unmarshal(data, msg); err != nil {
		return nil, status.NewErrorf(codes.CodeInvalidArgument, "decode stream message: %v", err)
	}
	return msg, nil
}

// Conn is the minimal WebSocket surface the streaming transports need.
type Conn interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, typ int, data []byte) error
	Close(code int, reason string) error
}