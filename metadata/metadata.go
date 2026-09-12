package metadata

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HTTP header names in canonical form, as used by net/http. The standard
// library does not export header-name constants (only methods and status
// codes), so the framework defines them here, in the transport-level
// package, instead of repeating string literals across runtime, client,
// server, and encoding.
const (
	HeaderAccept             = "Accept"
	HeaderAcceptEncoding     = "Accept-Encoding"
	HeaderAuthorization      = "Authorization"
	HeaderConnection         = "Connection"
	HeaderContentEncoding    = "Content-Encoding"
	HeaderContentLength      = "Content-Length"
	HeaderContentType        = "Content-Type"
	HeaderGrpcDeadline       = "Grpc-Deadline"
	HeaderGrpcStatusDetails  = "Grpc-Status-Details-Bin"
	HeaderGrpcTimeout        = "Grpc-Timeout"
	HeaderHost               = "Host"
	HeaderUpgrade            = "Upgrade"
	HeaderUserAgent          = "User-Agent"
)

// Common HTTP header values.
const (
	ConnectionKeepAlive = "keep-alive"
	ConnectionUpgrade   = "upgrade"
	ContentEncodingGzip = "gzip"
	UpgradeWebSocket    = "websocket"
)

// Common Content-Type media type values.
const (
	ContentTypeJSON        = "application/json"
	ContentTypeProtobuf    = "application/protobuf"
	ContentTypeXProtobuf   = "application/x-protobuf"
	ContentTypeXML         = "application/xml"
	ContentTypeEventStream = "text/event-stream"
	ContentTypeGRPC        = "application/grpc"
)

// RequestMeta carries the transport-level request data available to
// handlers: headers, query parameters, raw body, and cookies. It is
// populated from the HTTP request by the mux and from gRPC metadata by
// gRPC adapters, so handlers can read auth data (e.g. a JWT in the
// Authorization header) uniformly regardless of transport.
type RequestMeta struct {
	// Header holds the request headers. For HTTP requests this is the
	// incoming header set; for gRPC requests it is the incoming metadata
	// converted to http.Header form.
	Header http.Header
	// Query holds the URL query parameters (HTTP only).
	Query url.Values
	// Body holds the raw request body bytes. It is captured only for
	// single-message requests (unary and server-streaming); streaming
	// request bodies are consumed as streams and are not buffered.
	Body []byte
	// Cookies holds the request cookies (HTTP only).
	Cookies []*http.Cookie
	// Params holds the path parameters extracted from the route template
	// (HTTP only), e.g. {"id": "42"} for /v1/users/{id}. It is nil for
	// routes without path parameters.
	Params map[string]string
}

type requestMetaKey struct{}

// WithRequestMeta attaches request metadata to the context.
func WithRequestMeta(ctx context.Context, meta RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, meta)
}

// RequestMetaFromContext returns the request metadata attached to the
// context, or an empty RequestMeta.
func RequestMetaFromContext(ctx context.Context) RequestMeta {
	meta, _ := ctx.Value(requestMetaKey{}).(RequestMeta)
	return meta
}

// HeaderFromContext returns the request headers, or an empty header.
func HeaderFromContext(ctx context.Context) http.Header {
	if h := RequestMetaFromContext(ctx).Header; h != nil {
		return h
	}
	return make(http.Header)
}

// QueryFromContext returns the request query parameters, or an empty map.
func QueryFromContext(ctx context.Context) url.Values {
	if q := RequestMetaFromContext(ctx).Query; q != nil {
		return q
	}
	return make(url.Values)
}

// BodyFromContext returns the raw request body bytes. It is populated for
// unary and server-streaming requests; streaming request bodies are not
// buffered.
func BodyFromContext(ctx context.Context) []byte {
	return RequestMetaFromContext(ctx).Body
}

// CookiesFromContext returns the request cookies.
func CookiesFromContext(ctx context.Context) []*http.Cookie {
	return RequestMetaFromContext(ctx).Cookies
}

// ParamsFromContext returns the path parameters extracted from the route
// template, or nil when the route has no path parameters.
func ParamsFromContext(ctx context.Context) map[string]string {
	return RequestMetaFromContext(ctx).Params
}

// CookieFromContext returns the named cookie, or http.ErrNoCookie.
func CookieFromContext(ctx context.Context, name string) (*http.Cookie, error) {
	for _, c := range CookiesFromContext(ctx) {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, http.ErrNoCookie
}

// hopByHopHeaders are skipped when forwarding request headers to outgoing
// gRPC metadata (RFC 7230 §6.1). The generated gateway uses HeaderPairs to
// avoid forwarding transport-level headers that would confuse the gRPC
// server. grpc-timeout/grpc-deadline are skipped too: the deadline is
// forwarded through the context (the gateway passes ctx.Context() to the
// gRPC client, which computes its own grpc-timeout), so forwarding the raw
// header would double-apply it.
var hopByHopHeaders = [...]string{
	"content-type", "content-length", "connection", "upgrade",
	"accept", "accept-encoding", "user-agent", "host",
	"grpc-timeout", "grpc-deadline",
}

// HeaderPairs maps the request headers to a flat key/value slice suitable
// for grpc metadata.AppendToOutgoingContext, skipping hop-by-hop headers.
// It returns nil when there are no headers to forward, so callers can skip
// the grpc call entirely (AppendToOutgoingContext with zero pairs still
// wraps the context, allocating).
//
// Keys are lowercased: gRPC metadata keys are lowercase by convention and
// HTTP/2 header names must be lowercase on the wire, while net/http
// canonicalizes incoming header names to title case. The hop-by-hop
// comparison is allocation-free: EqualFold is case-insensitive without
// copying, whereas ToLower would allocate a new string for any uppercase
// key.
func HeaderPairs(ctx context.Context) []string {
	return HeaderPairsFromMeta(RequestMetaFromContext(ctx))
}

// HeaderPairsFromMeta is HeaderPairs for a RequestMeta. The generated
// gateway calls it with ctx.Meta() — the runtime.Ctx carries the metadata
// in its state, not in the wrapped context, so reading it back through
// context would be a wasted round trip.
func HeaderPairsFromMeta(meta RequestMeta) []string {
	h := meta.Header
	if len(h) == 0 {
		return nil
	}
	pairs := make([]string, 0, len(h)*2)
	for k, vs := range h {
		if isHopByHop(k) {
			continue
		}
		key := strings.ToLower(k)
		for _, v := range vs {
			pairs = append(pairs, key, v)
		}
	}
	if len(pairs) == 0 {
		return nil
	}
	return pairs
}

func isHopByHop(k string) bool {
	for _, h := range hopByHopHeaders {
		if strings.EqualFold(k, h) {
			return true
		}
	}
	return false
}

// maxTimeoutValue is the largest value the gRPC timeout wire format allows:
// 8 digits plus the unit (grpc-go's internal/grpcutil.maxTimeoutValue).
const maxTimeoutValue int64 = 100000000 - 1

// timeoutUnits maps the gRPC timeout unit suffix to a duration. The units
// are case-sensitive: H/M/S are hours/minutes/seconds, m/u/n are
// milliseconds/microseconds/nanoseconds.
var timeoutUnits = map[byte]time.Duration{
	'H': time.Hour,
	'M': time.Minute,
	'S': time.Second,
	'm': time.Millisecond,
	'u': time.Microsecond,
	'n': time.Nanosecond,
}

// FormatGrpcTimeout encodes a duration as a Grpc-Timeout header value,
// matching grpc-go's EncodeDuration: the smallest unit that fits in 8
// digits, rounding up. A non-positive duration encodes as "0n".
func FormatGrpcTimeout(d time.Duration) string {
	if d <= 0 {
		return "0n"
	}
	// div rounds up so the encoded value never under-represents the
	// deadline (a client that encodes 1ns as 1n must not lose it).
	div := func(d, r time.Duration) int64 {
		if d%r > 0 {
			return int64(d/r + 1)
		}
		return int64(d / r)
	}
	for _, unit := range []struct {
		d time.Duration
		s string
	}{
		{time.Nanosecond, "n"},
		{time.Microsecond, "u"},
		{time.Millisecond, "m"},
		{time.Second, "S"},
		{time.Minute, "M"},
		{time.Hour, "H"},
	} {
		if v := div(d, unit.d); v <= maxTimeoutValue {
			return strconv.FormatInt(v, 10) + unit.s
		}
	}
	return strconv.FormatInt(div(d, time.Hour), 10) + "H"
}

// ParseGrpcTimeout parses a Grpc-Timeout header value ("10S", "500m") into
// a duration. It matches grpc-go's decodeTimeout: 1-8 digits plus a unit
// suffix, integer only. Malformed values return ok=false so callers can
// ignore the header (the gRPC convention is to treat a bad timeout as no
// timeout).
func ParseGrpcTimeout(v string) (time.Duration, bool) {
	size := len(v)
	if size < 2 || size > 9 {
		return 0, false
	}
	unit, ok := timeoutUnits[v[size-1]]
	if !ok {
		return 0, false
	}
	t, err := strconv.ParseUint(v[:size-1], 10, 64)
	if err != nil {
		return 0, false
	}
	const maxHours = int64(^uint64(0)>>1) / int64(time.Hour)
	if unit == time.Hour && int64(t) > maxHours {
		// This timeout would overflow time.Duration; clamp it.
		return time.Duration(^uint64(0) >> 1), true
	}
	return unit * time.Duration(t), true
}

// ParseGrpcDeadline parses a Grpc-Deadline header value into a time.Time.
// The gRPC convention accepts an RFC3339 timestamp (the grpc-go
// grpc.WithTimeout-style deadline) or a Unix timestamp in seconds or
// nanoseconds. Malformed values return ok=false so callers can ignore the
// header.
func ParseGrpcDeadline(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		// Values with more than 10 digits are nanoseconds; otherwise
		// seconds. This mirrors grpc-go's deadline parsing.
		if len(v) > 10 {
			return time.Unix(0, n), true
		}
		return time.Unix(n, 0), true
	}
	return time.Time{}, false
}