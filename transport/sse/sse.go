// Package sse implements the SSE server-streaming transport.
package sse

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"sync"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/event"
	"github.com/aldok10/zara-rpc/internal/xsync"
	"github.com/aldok10/zara-rpc/status"
)

// Pooled buffers for framework-owned I/O. Safe to reuse because the data
// is decoded or copied out before the buffer returns to the pool (see the
// Codec contract: Unmarshal must not retain its input, Marshal returns a
// fresh slice). Buffers larger than 1 MiB are dropped, not retained.
var (
	// sseWriteBufPool holds base64 encode buffers for binary SSE events.
	sseWriteBufPool = xsync.NewBytes(1 << 20)
)

// ServerStream writes typed messages as SSE events. It implements
// runtime.ServerStream[any] and is used by server-streaming operations.
type ServerStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
	codec   encoding.Codec
	mu      sync.Mutex
}

// NewServerStream builds an SSE stream over the response writer.
func NewServerStream(w http.ResponseWriter, flusher http.Flusher, codec encoding.Codec) *ServerStream {
	return &ServerStream{w: w, flusher: flusher, codec: codec}
}

// Send writes one message as an SSE event. When the message is an
// *event.Event, the envelope fields are rendered as structured SSE fields
// (event:, id:, retry:) and Data is marshaled as the payload; otherwise a
// plain "data:" event is written.
func (s *ServerStream) Send(msg any) error {
	// Envelope handling: render the structured fields before the payload.
	var ev *event.Event
	if e, ok := msg.(*event.Event); ok {
		ev = e
		msg = e.Data
	}
	raw, err := s.codec.Marshal(msg)
	if err != nil {
		return status.NewErrorf(codes.CodeInternal, "encode stream message: %v", err)
	}
	data := raw
	// Binary codecs (e.g. protobuf) are base64-encoded so the payload stays
	// safe inside the SSE text framing. Encode into a pooled buffer to
	// avoid the string round-trip; the pool hands out *[]byte so Get/Put
	// are allocation-free.
	if s.codec.IsBinary() {
		enc := sseWriteBufPool.GetN(base64.StdEncoding.EncodedLen(len(raw)))
		base64.StdEncoding.Encode(*enc, raw)
		data = *enc
		defer sseWriteBufPool.Put(enc)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ev != nil {
		if ev.Name != "" {
			if _, err := s.w.Write([]byte("event: " + ev.Name + "\n")); err != nil {
				return err
			}
		}
		if ev.ID != "" {
			if _, err := s.w.Write([]byte("id: " + ev.ID + "\n")); err != nil {
				return err
			}
		}
		if ev.Retry > 0 {
			if _, err := s.w.Write([]byte("retry: " + strconv.Itoa(ev.Retry) + "\n")); err != nil {
				return err
			}
		}
	}
	if _, err := s.w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := s.w.Write(data); err != nil {
		return err
	}
	if _, err := s.w.Write([]byte("\n\n")); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}