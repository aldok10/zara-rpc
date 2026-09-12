// NDJSON client-streaming writer.
package client

import (
	"encoding/base64"
	"io"
	"net/http"
	"reflect"
	"sync"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/internal/xsync"
	"github.com/aldok10/zara-rpc/status"
)

// ndjsonWriteBufPool holds base64 encode buffers for binary NDJSON
// request lines.
var ndjsonWriteBufPool = xsync.NewBytes(1 << 20)

// ndjsonResult is the outcome of the client-streaming HTTP request.
type ndjsonResult struct {
	resp *http.Response
	err  error
}

// ndjsonWriteStream writes newline-delimited JSON requests for a
// client-streaming RPC.
type ndjsonWriteStream struct {
	w       io.Writer
	codec   encoding.Codec
	body    io.WriteCloser
	respCh  <-chan ndjsonResult
	respTyp reflect.Type

	mu     sync.Mutex
	result *ndjsonResult
}

// takeResult returns the request result if it has already arrived.
func (s *ndjsonWriteStream) takeResult() *ndjsonResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.result != nil {
		return s.result
	}
	select {
	case r := <-s.respCh:
		s.result = &r
	default:
	}
	return s.result
}

// waitResult blocks until the request result arrives.
func (s *ndjsonWriteStream) waitResult() *ndjsonResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.result != nil {
		return s.result
	}
	r := <-s.respCh
	s.result = &r
	return s.result
}

func (s *ndjsonWriteStream) Send(msg any) error {
	// If the server already responded (e.g. with an error), fail fast.
	if res := s.takeResult(); res != nil {
		if res.err != nil {
			return res.err
		}
		defer res.resp.Body.Close()
		return status.FromHTTP(res.resp)
	}
	raw, err := s.codec.Marshal(msg)
	if err != nil {
		return status.NewErrorf(codes.CodeInternal, "encode request: %v", err)
	}
	data := raw
	// NDJSON is newline-delimited: every message must end with '\n' or the
	// server's line reader concatenates messages into one line and sees
	// EOF before any complete message. Binary codecs (e.g. protobuf) are
	// base64-encoded per line; encode into a pooled buffer (with room for
	// the trailing newline) instead of the string round-trip, which
	// allocated twice per message. The pool hands out *[]byte so Get/Put
	// are allocation-free.
	if s.codec.IsBinary() {
		n := base64.StdEncoding.EncodedLen(len(raw))
		enc := ndjsonWriteBufPool.GetN(n + 1)
		base64.StdEncoding.Encode(*enc, raw)
		(*enc)[n] = '\n'
		data = (*enc)[:n+1]
		defer ndjsonWriteBufPool.Put(enc)
	} else {
		// Marshal returns a fresh slice, so appending is safe.
		data = append(raw, '\n')
	}
	if _, err := s.w.Write(data); err != nil {
		return err
	}
	return nil
}

func (s *ndjsonWriteStream) CloseAndReceive() (any, error) {
	if err := s.body.Close(); err != nil {
		return nil, err
	}
	res := s.waitResult()
	if res.err != nil {
		return nil, res.err
	}
	defer res.resp.Body.Close()
	if res.resp.StatusCode != http.StatusOK {
		return nil, status.FromHTTP(res.resp)
	}
	msg := reflect.New(s.respTyp).Interface()
	if err := s.codec.NewDecoder(res.resp.Body).Decode(msg); err != nil {
		return nil, status.NewErrorf(codes.CodeInternal, "decode response: %v", err)
	}
	return msg, nil
}

func (s *ndjsonWriteStream) Close() error {
	return s.body.Close()
}
