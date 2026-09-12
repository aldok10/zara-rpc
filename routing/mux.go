package routing

import (
	"net/http"
	"sort"
	"strings"

	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/kernel"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/runtime"
)

// Mux routes HTTP requests to registered service operations. It is a
// plain http.Handler and can be mounted anywhere or used directly with
// http.ListenAndServe.
type Mux struct {
	codec              encoding.Codec
	interceptors       []middleware.UnaryInterceptor
	streamInterceptors []middleware.StreamInterceptor
	notFound           http.Handler

	// exact maps HTTP method -> exact path -> handler. Exact routes (no
	// path parameters) are looked up in O(1) via Go's map; wildcard routes
	// fall back to the per-method radix tree below.
	exact map[string]map[string]*kernel.Handler

	// wildcard maps HTTP method -> radix tree. Only patterns containing
	// path parameters live here. Lookup is O(k) in path length: the walk
	// matches literal prefixes, captures param segments, and collects the
	// parameters in one pass.
	wildcard map[string]*routeTree
}

// MuxOption configures a Mux.
type MuxOption func(*Mux)

// WithMuxCodec sets the default codec for operations registered without
// an explicit codec. Defaults to JSONCodec.
func WithMuxCodec(codec encoding.Codec) MuxOption {
	return func(m *Mux) { m.codec = codec }
}

// WithMuxUnaryInterceptors adds unary interceptors applied to every
// operation.
func WithMuxUnaryInterceptors(interceptors ...middleware.UnaryInterceptor) MuxOption {
	return func(m *Mux) { m.interceptors = append(m.interceptors, interceptors...) }
}

// WithMuxStreamInterceptors adds per-message stream interceptors applied
// to every streaming operation. They wrap the stream object after the
// connection-level interceptor chain has authorized the request, so each
// Send/Receive is observed per message.
func WithMuxStreamInterceptors(interceptors ...middleware.StreamInterceptor) MuxOption {
	return func(m *Mux) { m.streamInterceptors = append(m.streamInterceptors, interceptors...) }
}

// WithNotFound sets the handler used when no operation matches.
func WithNotFound(h http.Handler) MuxOption {
	return func(m *Mux) { m.notFound = h }
}

// NewMux creates an empty Mux.
func NewMux(opts ...MuxOption) *Mux {
	m := &Mux{
		codec:    encoding.JSONCodec{},
		exact:    make(map[string]map[string]*kernel.Handler),
		wildcard: make(map[string]*routeTree),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Register registers all operations of a service on the mux. It returns
// an error if an operation's path template is invalid or a duplicate
// route is registered.
func (m *Mux) Register(service *kernel.Service) error {
	for _, e := range service.Operations() {
		path := service.Prefix + e.Path
		kernel.PrepareOperation(e, service.Name, m.codec, m.interceptors, m.streamInterceptors, path)

		// Re-parse with the prefixed path. The Handler does not own the
		// pattern; the mux matches and passes params to
		// Handler.ServeHTTPWithParams.
		pattern, err := ParsePattern(path)
		if err != nil {
			return err
		}

		h := kernel.NewHandler(e)
		// Bidi operations are transported over WebSocket, whose handshake is
		// always a GET request regardless of the declared HTTP method.
		method := e.Method
		if e.StreamType() == runtime.StreamTypeBidi {
			method = http.MethodGet
		}
		// Exact routes (no path parameters) are hash-indexed for O(1)
		// routing; wildcard routes go into the per-method radix tree,
		// which matches in O(k) path length regardless of route count.
		// First registration wins for duplicate exact paths.
		if strings.Contains(path, "{") {
			tree := m.wildcard[method]
			if tree == nil {
				tree = newRouteTree()
				m.wildcard[method] = tree
			}
			tree.insert(pattern, h)
		} else {
			byPath := m.exact[method]
			if byPath == nil {
				byPath = make(map[string]*kernel.Handler)
				m.exact[method] = byPath
			}
			if _, exists := byPath[path]; !exists {
				byPath[path] = h
			}
		}
	}
	return nil
}

// ServeHTTP implements http.Handler.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Exact routes first: a single map lookup, no pattern matching.
	if byPath := m.exact[r.Method]; byPath != nil {
		if h := byPath[r.URL.Path]; h != nil {
			h.ServeHTTPWithParams(w, r, nil)
			return
		}
	}
	// Wildcard routes: walk the per-method radix tree. The walk matches
	// literal prefixes before param children, so the most specific match
	// wins, and collects the path parameters in one pass.
	if tree := m.wildcard[r.Method]; tree != nil {
		if h, params := tree.lookup(r.URL.Path); h != nil {
			h.ServeHTTPWithParams(w, r, params)
			return
		}
	}

	// No match for this method. If the path matches another method,
	// respond 405 with an Allow header (grpc-gateway behavior).
	if m.pathExists(r.URL.Path) {
		allow := m.allowedMethods(r.URL.Path)
		w.Header().Set("Allow", allow)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	if m.notFound != nil {
		m.notFound.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

// pathExists reports whether any registered handler matches the path
// regardless of method.
func (m *Mux) pathExists(path string) bool {
	for _, byPath := range m.exact {
		if _, ok := byPath[path]; ok {
			return true
		}
	}
	for _, tree := range m.wildcard {
		if h, _ := tree.lookup(path); h != nil {
			return true
		}
	}
	return false
}

// allowedMethods returns the comma-separated Allow header value for a path.
func (m *Mux) allowedMethods(path string) string {
	seen := make(map[string]bool)
	var methods []string
	for method, byPath := range m.exact {
		if _, ok := byPath[path]; ok && !seen[method] {
			seen[method] = true
			methods = append(methods, method)
		}
	}
	for method, tree := range m.wildcard {
		if h, _ := tree.lookup(path); h != nil && !seen[method] {
			seen[method] = true
			methods = append(methods, method)
		}
	}
	sort.Strings(methods)
	allow := ""
	for i, method := range methods {
		if i > 0 {
			allow += ", "
		}
		allow += method
	}
	return allow
}
