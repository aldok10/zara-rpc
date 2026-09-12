package encoding

import (
	"fmt"
	"io"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/aldok10/zara-rpc/internal/xsync"
	"github.com/aldok10/zara-rpc/metadata"
)

// ProtoCodec serializes protobuf messages in their native binary wire
// format. It is selected when a request carries Content-Type
// "application/protobuf" (or "application/x-protobuf").
type ProtoCodec struct{}

func (ProtoCodec) Name() string { return metadata.ContentTypeProtobuf }

// IsBinary reports that proto output is not safe for text-based framing
// (SSE, NDJSON); transports must base64-encode it.
func (ProtoCodec) IsBinary() bool { return true }

func (ProtoCodec) Marshal(v any) ([]byte, error) {
	pm, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("ProtoCodec: %T does not implement proto.Message", v)
	}
	return proto.Marshal(pm)
}

func (ProtoCodec) Unmarshal(data []byte, v any) error {
	pm, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("ProtoCodec: %T does not implement proto.Message", v)
	}
	return proto.Unmarshal(data, pm)
}

func (ProtoCodec) NewDecoder(r io.Reader) Decoder {
	return &protoDecoder{r: r}
}

func (ProtoCodec) NewEncoder(w io.Writer) Encoder {
	return &protoEncoder{w: w}
}

// protoReadBufPool reuses the request-body read buffer across decodes.
// proto.Unmarshal copies wire data into the message (bytes fields are
// never aliased — guarded by TestProtoDecoderBufferNotAliased), so the
// buffer can be returned to the pool as soon as unmarshal completes.
var protoReadBufPool = xsync.NewBuffer(64 << 10)

// protoDecoder reads the entire stream and unmarshals it as one message.
type protoDecoder struct {
	r io.Reader
}

func (d *protoDecoder) Decode(v any) error {
	pm, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("ProtoCodec: %T does not implement proto.Message", v)
	}
	buf := protoReadBufPool.Get()
	defer protoReadBufPool.Put(buf)
	if _, err := buf.ReadFrom(d.r); err != nil {
		return err
	}
	return proto.Unmarshal(buf.Bytes(), pm)
}

// protoEncoder marshals one message and writes the bytes.
type protoEncoder struct {
	w io.Writer
}

func (e *protoEncoder) Encode(v any) error {
	pm, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("ProtoCodec: %T does not implement proto.Message", v)
	}
	data, err := proto.Marshal(pm)
	if err != nil {
		return err
	}
	_, err = e.w.Write(data)
	return err
}

// CodecForContentType returns the codec for a Content-Type header value.
// The media type is parsed (ignoring parameters such as charset) and
// mapped to the matching codec; unknown media types fall back to JSON.
//
// The comparison is allocation-free: EqualFold is case-insensitive without
// copying, whereas ToLower would allocate a new string for any uppercase
// input (e.g. "Application/JSON").
func CodecForContentType(ct string) Codec {
	mediaType := ct
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		mediaType = ct[:i]
	}
	mediaType = strings.TrimSpace(mediaType)
	switch {
	case strings.EqualFold(mediaType, metadata.ContentTypeProtobuf),
		strings.EqualFold(mediaType, metadata.ContentTypeXProtobuf):
		return ProtoCodec{}
	default:
		return JSONCodec{}
	}
}

// CodecForRequest negotiates the codec for a request. Content-Type
// describes the request body format and wins when present; for bodyless
// requests (GET/DELETE) the Accept header selects the response format.
func CodecForRequest(ct, accept string) Codec {
	if ct != "" {
		return CodecForContentType(ct)
	}
	if accept != "" && accept != "*/*" {
		return CodecForContentType(accept)
	}
	return JSONCodec{}
}