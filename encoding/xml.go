package encoding

import (
	"encoding/xml"
	"io"

	"github.com/aldok10/zara-rpc/metadata"
)

// XMLCodec serializes messages as XML using encoding/xml. Proto messages
// are not supported by this codec.
type XMLCodec struct{}

func (XMLCodec) Name() string { return metadata.ContentTypeXML }

// IsBinary reports that XML output is text-safe.
func (XMLCodec) IsBinary() bool { return false }

func (XMLCodec) Marshal(v any) ([]byte, error) {
	return xml.Marshal(v)
}

func (XMLCodec) Unmarshal(data []byte, v any) error {
	return xml.Unmarshal(data, v)
}

func (XMLCodec) NewDecoder(r io.Reader) Decoder {
	return xml.NewDecoder(r)
}

func (XMLCodec) NewEncoder(w io.Writer) Encoder {
	return xml.NewEncoder(w)
}