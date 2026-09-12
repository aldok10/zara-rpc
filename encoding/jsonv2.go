package encoding

import (
	"io"

	jsonv2 "encoding/json/v2"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// JSONV2Codec uses the experimental JSON v2 package
// (encoding/json/v2) for faster, more correct serialization.
// Like JSONCodec it transparently handles proto messages.
type JSONV2Codec struct{}

func (JSONV2Codec) Name() string { return "application/json.v2" }

// IsBinary reports that JSON output is text-safe.
func (JSONV2Codec) IsBinary() bool { return false }

func (JSONV2Codec) Marshal(v any) ([]byte, error) {
	if pm, ok := v.(proto.Message); ok {
		return protojson.Marshal(pm)
	}
	return jsonv2.Marshal(v)
}

func (JSONV2Codec) Unmarshal(data []byte, v any) error {
	if pm, ok := v.(proto.Message); ok {
		return protojson.Unmarshal(data, pm)
	}
	return jsonv2.Unmarshal(data, v)
}

func (JSONV2Codec) NewDecoder(r io.Reader) Decoder {
	return &jsonv2Decoder{r: r}
}

func (JSONV2Codec) NewEncoder(w io.Writer) Encoder {
	return &jsonv2Encoder{w: w}
}

// jsonv2Decoder wraps UnmarshalRead for single-value streaming decode.
type jsonv2Decoder struct {
	r io.Reader
}

func (d *jsonv2Decoder) Decode(v any) error {
	return jsonv2.UnmarshalRead(d.r, v)
}

// jsonv2Encoder wraps MarshalWrite for single-value streaming encode.
type jsonv2Encoder struct {
	w io.Writer
}

func (e *jsonv2Encoder) Encode(v any) error {
	return jsonv2.MarshalWrite(e.w, v)
}
