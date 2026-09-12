package runtime

import "errors"

// StreamType describes the RPC streaming mode.
type StreamType int

const (
	// StreamTypeUnary is a single request / single response RPC.
	StreamTypeUnary StreamType = iota
	// StreamTypeServer is a single request / streamed responses RPC
	// (transported as SSE).
	StreamTypeServer
	// StreamTypeClient is a streamed requests / single response RPC
	// (transported as newline-delimited JSON).
	StreamTypeClient
	// StreamTypeBidi is a streamed requests / streamed responses RPC
	// (transported over WebSocket).
	StreamTypeBidi
)

// String returns the stream type name.
func (s StreamType) String() string {
	switch s {
	case StreamTypeServer:
		return "server"
	case StreamTypeClient:
		return "client"
	case StreamTypeBidi:
		return "bidi"
	default:
		return "unary"
	}
}

// ServerStream is the server-side view of a server-streaming RPC.
type ServerStream[T any] interface {
	Send(T) error
}

// ClientStream is the server-side view of a client-streaming RPC.
type ClientStream[T any] interface {
	// Receive reads the next request message. It returns io.EOF when the
	// client has finished sending.
	Receive() (T, error)
}

// BidiStream is the server-side view of a bidi-streaming RPC.
type BidiStream[Req, Resp any] interface {
	Send(Resp) error
	// Receive reads the next request message. It returns io.EOF when the
	// client has finished sending.
	Receive() (Req, error)
}

// ErrStreamClosed is returned when operating on a closed stream.
var ErrStreamClosed = errors.New("stream closed")
