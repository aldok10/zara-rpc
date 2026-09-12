// Package ndjson implements the NDJSON client-streaming transport.
package ndjson

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"reflect"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/internal/xsync"
	"github.com/aldok10/zara-rpc/status"
)

var (
	// ndjsonReadBufPool holds base64 decode buffers for binary NDJSON
	// request lines.
	ndjsonReadBufPool = xsync.NewBytes(1 << 20)
)

// ClientStream reads newline-delimited JSON messages from the request
// body. It implements runtime.ClientStream[any] and is used by
// client-streaming operations.
type ClientStream struct {
	r      *bufio.Reader
	codec  encoding.Codec
	msgTyp reflect.Type
}

// NewClientStream builds an NDJSON stream over the request body.
func NewClientStream(r io.Reader, codec encoding.Codec, msgTyp reflect.Type) *ClientStream {
	return &ClientStream{r: bufio.NewReader(r), codec: codec, msgTyp: msgTyp}
}

// Receive reads the next request message.
func (s *ClientStream) Receive() (any, error) {
	line, err := s.r.ReadBytes('\n')
	if err != nil {
		if err == io.EOF && len(bytes.TrimSpace(line)) == 0 {
			return nil, io.EOF
		}

		return nil, err
	}

	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, io.EOF
	}

	// Binary codecs (e.g. protobuf) are base64-encoded per line. Decode
	// into a pooled buffer to avoid the string round-trip; the decode
	// buffer is framework-owned and dead after Unmarshal.
	var pooled *[]byte
	if s.codec.IsBinary() {
		decoded := ndjsonReadBufPool.GetN(base64.StdEncoding.DecodedLen(len(line)))
		n, err := base64.StdEncoding.Decode(*decoded, line)
		if err != nil {
			ndjsonReadBufPool.Put(decoded)
			return nil, status.NewErrorf(codes.CodeInvalidArgument, "decode base64 stream message: %v", err)
		}
		line = (*decoded)[:n]
		pooled = decoded
	}

	msg := reflect.New(s.msgTyp).Interface()
	if err := s.codec.Unmarshal(line, msg); err != nil {
		if pooled != nil {
			ndjsonReadBufPool.Put(pooled)
		}
		return nil, status.NewErrorf(codes.CodeInvalidArgument, "decode stream message: %v", err)
	}
	if pooled != nil {
		ndjsonReadBufPool.Put(pooled)
	}
	return msg, nil
}