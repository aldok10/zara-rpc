package client

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/middleware"
)

// ClientOption configures a generated client.
type ClientOption func(*clientConfig)

// ServerStreamTransport selects the transport for server-streaming RPCs.
type ServerStreamTransport int

const (
	// ServerStreamSSE streams responses as Server-Sent Events (default).
	ServerStreamSSE ServerStreamTransport = iota
	// ServerStreamWebSocket streams responses over a WebSocket.
	ServerStreamWebSocket
)

type clientConfig struct {
	codec                 encoding.Codec
	httpClient            *http.Client
	headers               http.Header
	reqType               reflect.Type
	respType              reflect.Type
	serverStreamTransport ServerStreamTransport
	streamInterceptors    []middleware.StreamInterceptor
	unaryInterceptors     []UnaryInterceptor
	retryPolicy           *RetryPolicy
	timeout               time.Duration
	deadline              time.Time
}

func defaultClientConfig() clientConfig {
	return clientConfig{
		codec:                 encoding.JSONCodec{},
		httpClient:            http.DefaultClient,
		headers:               make(http.Header),
		serverStreamTransport: ServerStreamSSE,
	}
}

// WithCodec sets the codec used to encode requests and decode responses.
// Defaults to JSONCodec.
func WithCodec(c encoding.Codec) ClientOption {
	return func(cfg *clientConfig) { cfg.codec = c }
}

// WithHTTPClient sets the underlying HTTP client. Defaults to
// http.DefaultClient.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(cfg *clientConfig) { cfg.httpClient = hc }
}

// WithHeader adds a header to every request.
func WithHeader(k, v string) ClientOption {
	return func(cfg *clientConfig) { cfg.headers.Add(k, v) }
}

// WithServerStreamTransport sets the transport used by server-streaming
// RPCs. Defaults to ServerStreamSSE.
func WithServerStreamTransport(t ServerStreamTransport) ClientOption {
	return func(cfg *clientConfig) { cfg.serverStreamTransport = t }
}

// WithStreamInterceptors adds per-message stream interceptors that wrap
// each Send/Receive on streaming RPCs. They run after the HTTP request is
// established (so the connection-level status is already resolved).
func WithStreamInterceptors(interceptors ...middleware.StreamInterceptor) ClientOption {
	return func(cfg *clientConfig) { cfg.streamInterceptors = append(cfg.streamInterceptors, interceptors...) }
}

// WithUnaryInterceptors adds unary interceptors applied to every unary
// call. They wrap the HTTP round trip; the first interceptor is the
// outermost. Retries (WithRetryPolicy) wrap the interceptor chain, so
// interceptors observe each attempt.
func WithUnaryInterceptors(interceptors ...UnaryInterceptor) ClientOption {
	return func(cfg *clientConfig) { cfg.unaryInterceptors = append(cfg.unaryInterceptors, interceptors...) }
}

// WithRetryPolicy enables retries for unary calls. Retries apply only to
// idempotent methods (GET/HEAD or RetryPolicy.Idempotent) and only to
// CodeUnavailable failures. The policy is copied, so later mutation of the
// caller's value has no effect.
func WithRetryPolicy(p RetryPolicy) ClientOption {
	return func(cfg *clientConfig) {
		p.normalize()
		cfg.retryPolicy = &p
	}
}

// WithTimeout sets a per-call timeout. It sets the Grpc-Timeout header (so
// the server honors the deadline) and applies a local context deadline (so
// the client cancels when the server exceeds it). WithTimeout wins over
// WithDeadline when both are set.
func WithTimeout(d time.Duration) ClientOption {
	return func(cfg *clientConfig) { cfg.timeout = d }
}

// WithDeadline sets an absolute per-call deadline. It sets the
// Grpc-Deadline header and applies a local context deadline.
func WithDeadline(t time.Time) ClientOption {
	return func(cfg *clientConfig) { cfg.deadline = t }
}

// applyDeadline sets the deadline header and derives the local context
// deadline for a call. It must be called after per-call options are applied
// (the header map is the cloned per-call map, so the base client is never
// mutated). The returned cancel func must be called when the call finishes.
func (cfg *clientConfig) applyDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if cfg.timeout > 0 {
		cfg.headers.Set(metadata.HeaderGrpcTimeout, metadata.FormatGrpcTimeout(cfg.timeout))
		return context.WithTimeout(ctx, cfg.timeout)
	}
	if !cfg.deadline.IsZero() {
		cfg.headers.Set(metadata.HeaderGrpcDeadline, cfg.deadline.Format(time.RFC3339))
		return context.WithDeadline(ctx, cfg.deadline)
	}
	return ctx, func() {}
}

// ClientBase is embedded by generated clients. It holds the shared
// configuration and provides the call helpers.
type ClientBase struct {
	baseURL string
	cfg     clientConfig
}

// NewClientBase builds the shared client state.
func NewClientBase(baseURL string, opts ...ClientOption) ClientBase {
	cfg := defaultClientConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return ClientBase{baseURL: baseURL, cfg: cfg}
}
