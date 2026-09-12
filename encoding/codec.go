package encoding

import (
	"io"
	"sync"
)

// Codec serializes and deserializes request and response messages.
type Codec interface {
	// Name returns the codec name, used in Content-Type negotiation.
	Name() string
	// Marshal encodes a message into bytes. The returned slice is fresh
	// (never aliases v) and the caller may reuse or pool it after writing.
	Marshal(v any) ([]byte, error)
	// Unmarshal decodes bytes into a message. The implementation must not
	// retain data: the caller may reuse the buffer immediately after
	// Unmarshal returns.
	Unmarshal(data []byte, v any) error
	// NewDecoder returns a decoder for a stream.
	NewDecoder(r io.Reader) Decoder
	// NewEncoder returns an encoder for a stream.
	NewEncoder(w io.Writer) Encoder
	// IsBinary reports whether the codec produces binary output that is
	// not safe inside text-based framing (SSE, NDJSON). Binary codecs are
	// base64-encoded on text transports and use binary frames on
	// WebSocket.
	IsBinary() bool
}

// Decoder decodes values from a stream.
type Decoder interface {
	Decode(v any) error
}

// Encoder encodes values to a stream.
type Encoder interface {
	Encode(v any) error
}

// Registry maps codec names to codecs. It is safe for concurrent use.
type Registry struct {
	mu     sync.RWMutex
	codecs map[string]Codec
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{codecs: make(map[string]Codec)}
}

// Register adds a codec under its Name. Registering an existing name
// replaces the previous codec.
func (r *Registry) Register(c Codec) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.codecs[c.Name()] = c
}

// Get returns the codec registered under name.
func (r *Registry) Get(name string) (Codec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.codecs[name]
	return c, ok
}

// DefaultRegistry holds the built-in codecs.
var DefaultRegistry = NewRegistry()

func init() {
	DefaultRegistry.Register(JSONCodec{})
	DefaultRegistry.Register(JSONV2Codec{})
	DefaultRegistry.Register(XMLCodec{})
	DefaultRegistry.Register(ProtoCodec{})
}

// ForName returns a codec by name from the default registry, falling back
// to JSONCodec when the name is unknown.
func ForName(name string) Codec {
	if c, ok := DefaultRegistry.Get(name); ok {
		return c
	}
	return JSONCodec{}
}