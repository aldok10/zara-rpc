package encoding

import (
	"bytes"
	"testing"

	"go.uber.org/goleak"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// benchUser is a representative plain struct for text codec benchmarks:
// strings, an int, a repeated string, and a nested struct. XML tags are
// included so the XML codec renders the same shape.
type benchUser struct {
	ID       string     `json:"id" xml:"id"`
	Name     string     `json:"name" xml:"name"`
	Email    string     `json:"email" xml:"email"`
	Age      int        `json:"age" xml:"age"`
	Tags     []string   `json:"tags" xml:"tags>tag"`
	Address  benchAddr  `json:"address" xml:"address"`
	Contacts []benchCtx `json:"contacts" xml:"contacts>contact"`
}

type benchAddr struct {
	Street string `json:"street" xml:"street"`
	City   string `json:"city" xml:"city"`
	Zip    string `json:"zip" xml:"zip"`
}

type benchCtx struct {
	Type  string `json:"type" xml:"type,attr"`
	Value string `json:"value" xml:"value"`
}

func benchUserMsg() benchUser {
	return benchUser{
		ID:    "users/1",
		Name:  "Ada Lovelace",
		Email: "ada@example.com",
		Age:   36,
		Tags:  []string{"admin", "billing", "beta"},
		Address: benchAddr{
			Street: "12 Analytical Engine Way",
			City:   "London",
			Zip:    "SW1A 1AA",
		},
		Contacts: []benchCtx{
			{Type: "email", Value: "ada@example.com"},
			{Type: "phone", Value: "+44 20 7946 0958"},
		},
	}
}

// --- JSON (encoding/json) ---

func BenchmarkJSONMarshal(b *testing.B) {
	codec := JSONCodec{}
	msg := benchUserMsg()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := codec.Marshal(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONUnmarshal(b *testing.B) {
	codec := JSONCodec{}
	data, err := codec.Marshal(benchUserMsg())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var msg benchUser
		if err := codec.Unmarshal(data, &msg); err != nil {
			b.Fatal(err)
		}
	}
}

// --- JSON v2 (encoding/json/v2) ---

func BenchmarkJSONV2Marshal(b *testing.B) {
	codec := JSONV2Codec{}
	msg := benchUserMsg()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := codec.Marshal(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONV2Unmarshal(b *testing.B) {
	codec := JSONV2Codec{}
	data, err := codec.Marshal(benchUserMsg())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var msg benchUser
		if err := codec.Unmarshal(data, &msg); err != nil {
			b.Fatal(err)
		}
	}
}

// --- XML ---

func BenchmarkXMLMarshal(b *testing.B) {
	codec := XMLCodec{}
	msg := benchUserMsg()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := codec.Marshal(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkXMLUnmarshal(b *testing.B) {
	codec := XMLCodec{}
	data, err := codec.Marshal(benchUserMsg())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var msg benchUser
		if err := codec.Unmarshal(data, &msg); err != nil {
			b.Fatal(err)
		}
	}
}

// --- protojson production path ---
//
// JSONCodec.Marshal switches to protojson for proto.Message values. This
// is the path the mux takes for generated protobuf handlers with the
// default JSON codec — the plain encoding/json path never runs for them.

func BenchmarkJSONProtoMarshal(b *testing.B) {
	codec := JSONCodec{}
	msg := benchProtoMsg()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := codec.Marshal(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONProtoUnmarshal(b *testing.B) {
	codec := JSONCodec{}
	data, err := codec.Marshal(benchProtoMsg())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		msg := &descriptorpb.FileDescriptorProto{}
		if err := codec.Unmarshal(data, msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONV2ProtoMarshal(b *testing.B) {
	codec := JSONV2Codec{}
	msg := benchProtoMsg()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := codec.Marshal(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONV2ProtoUnmarshal(b *testing.B) {
	codec := JSONV2Codec{}
	data, err := codec.Marshal(benchProtoMsg())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		msg := &descriptorpb.FileDescriptorProto{}
		if err := codec.Unmarshal(data, msg); err != nil {
			b.Fatal(err)
		}
	}
}

// benchProtoMsg returns a representative nested message for codec
// benchmarks: strings, repeated strings, and a nested message with fields.
func benchProtoMsg() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:       proto.String("users/v1/users.proto"),
		Package:    proto.String("acme.users.v1"),
		Dependency: []string{"google/api/annotations.proto", "google/protobuf/empty.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("User"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("id"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
					{Name: proto.String("name"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
					{Name: proto.String("email"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
				},
			},
		},
	}
}

func BenchmarkProtoMarshal(b *testing.B) {
	msg := benchProtoMsg()
	codec := ProtoCodec{}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := codec.Marshal(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProtoUnmarshal(b *testing.B) {
	codec := ProtoCodec{}
	data, err := codec.Marshal(benchProtoMsg())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		msg := &descriptorpb.FileDescriptorProto{}
		if err := codec.Unmarshal(data, msg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProtoDecoder exercises the full request-body path: read from an
// io.Reader, then unmarshal. This is where the pooled read buffer applies.
func BenchmarkProtoDecoder(b *testing.B) {
	codec := ProtoCodec{}
	data, err := codec.Marshal(benchProtoMsg())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		dec := codec.NewDecoder(bytes.NewReader(data))
		msg := &descriptorpb.FileDescriptorProto{}
		if err := dec.Decode(msg); err != nil {
			b.Fatal(err)
		}
	}
}

// TestProtoDecoderBufferNotAliased guards the pooled-buffer optimization:
// proto.Unmarshal must copy bytes fields so a reused read buffer can never
// be referenced by a decoded message. If protobuf-go ever starts aliasing,
// this test fails and the pool must be removed.
func TestProtoDecoderBufferNotAliased(t *testing.T) {
	codec := ProtoCodec{}
	dec := codec.NewDecoder(bytes.NewReader([]byte{0x0a, 0x03, 0x01, 0x02, 0x03})) // BytesValue{value: [1,2,3]}
	msg := &wrapperspb.BytesValue{}
	if err := dec.Decode(msg); err != nil {
		t.Fatal(err)
	}
	// A second Decode reuses the pooled read buffer; the first message must
	// not observe that reuse.
	dec2 := codec.NewDecoder(bytes.NewReader([]byte{0x0a, 0x01, 0x09})) // BytesValue{value: [9]}
	msg2 := &wrapperspb.BytesValue{}
	if err := dec2.Decode(msg2); err != nil {
		t.Fatal(err)
	}
	if len(msg.Value) != 3 || msg.Value[0] != 1 || msg.Value[2] != 3 {
		t.Fatalf("first message corrupted by buffer reuse: %v", msg.Value)
	}
}
