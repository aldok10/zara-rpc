package kernel

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"time"
)

// Server wraps a standard net/http.Server with zara-rpc-friendly
// defaults and lifecycle management.
type Server struct {
	inner *http.Server
	addr  string
}

// Option configures the server.
type Option func(*Server)

// WithReadTimeout sets the server read timeout.
func WithReadTimeout(d time.Duration) Option {
	return func(s *Server) { s.inner.ReadTimeout = d }
}

// WithWriteTimeout sets the server write timeout.
func WithWriteTimeout(d time.Duration) Option {
	return func(s *Server) { s.inner.WriteTimeout = d }
}

// WithIdleTimeout sets the server idle timeout.
func WithIdleTimeout(d time.Duration) Option {
	return func(s *Server) { s.inner.IdleTimeout = d }
}

// WithTLS configures TLS with the given certificate and key.
func WithTLS(certFile, keyFile string) Option {
	return func(s *Server) {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Fatalf("server: load TLS keypair: %v", err)
		}
		s.inner.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}
}

// WithMaxHeaderBytes sets the maximum header size.
func WithMaxHeaderBytes(n int) Option {
	return func(s *Server) { s.inner.MaxHeaderBytes = n }
}

// New creates a new Server. The addr is the address to listen on
// (e.g. ":8080" or "0.0.0.0:8080"). handler is any http.Handler —
// typically a *routing.Mux.
func New(addr string, handler http.Handler, opts ...Option) *Server {
	s := &Server{
		inner: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
		},
		addr: addr,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ListenAndServe starts the server. It blocks until the server is shut down.
func (s *Server) ListenAndServe() error {
	if s.inner.TLSConfig != nil {
		return s.inner.ListenAndServeTLS("", "")
	}
	return s.inner.ListenAndServe()
}

// Shutdown gracefully shuts down the server, waiting for pending requests.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.inner.Shutdown(ctx)
}

// Inner returns the underlying http.Server for advanced configuration.
func (s *Server) Inner() *http.Server {
	return s.inner
}
