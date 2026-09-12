package encoding

import (
	"encoding/json"
	"io"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/aldok10/zara-rpc/metadata"
)

// JSONCodec is the default codec using encoding/json. When the value
// implements proto.Message, it transparently switches to protojson so
// protobuf field names, enums, and well-known types render correctly.
type JSONCodec struct{}

func (JSONCodec) Name() string { return metadata.ContentTypeJSON }

// IsBinary reports that JSON output is text-safe.
func (JSONCodec) IsBinary() bool { return false }

func (JSONCodec) Marshal(v any) ([]byte, error) {
	if pm, ok := v.(proto.Message); ok {
		return protojson.Marshal(pm)
	}
	return json.Marshal(v)
}

func (JSONCodec) Unmarshal(data []byte, v any) error {
	if pm, ok := v.(proto.Message); ok {
		return protojson.Unmarshal(data, pm)
	}
	return json.Unmarshal(data, v)
}

func (JSONCodec) NewDecoder(r io.Reader) Decoder {
	return json.NewDecoder(r)
}

func (JSONCodec) NewEncoder(w io.Writer) Encoder {
	return json.NewEncoder(w)
}