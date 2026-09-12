package streaming

// TransportRegistry maps stream transports to their names. It is the
// single place where a transport registers itself so the framework can
// negotiate codecs and transports without importing transport packages
// directly (avoiding cycles). The registry is intentionally minimal: it
// holds names and constructors, not lifecycle state.
type TransportRegistry struct {
	transports map[string]Transport
}

// Transport is a stream transport registered in the registry.
type Transport interface {
	// Name is the transport name, e.g. "sse", "ndjson", "websocket".
	Name() string
}

// NewTransportRegistry creates an empty registry.
func NewTransportRegistry() *TransportRegistry {
	return &TransportRegistry{transports: make(map[string]Transport)}
}

// Register adds a transport. Later registrations with the same name win.
func (r *TransportRegistry) Register(t Transport) {
	r.transports[t.Name()] = t
}

// Get returns the transport by name, or nil.
func (r *TransportRegistry) Get(name string) Transport {
	return r.transports[name]
}

// Names returns the registered transport names, sorted.
func (r *TransportRegistry) Names() []string {
	names := make([]string, 0, len(r.transports))
	for name := range r.transports {
		names = append(names, name)
	}
	return names
}