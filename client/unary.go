// Unary call helper.
package client

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

// DoUnary performs a unary RPC call. Path parameters in the path template
// are substituted from the request message. body is the body mapping from
// the HTTP rule: "*" (whole message), "field" (single field), or "" (no
// body; remaining fields become query parameters).
//
// Per-call options (WithTimeout, WithDeadline, WithRetryPolicy,
// WithUnaryInterceptors, WithHeader, ...) apply to this
// call only and never mutate the base client.
func (c *ClientBase) DoUnary(ctx context.Context, method, path, body string, req, resp any, opts ...ClientOption) error {
	cfg := c.cfg
	if len(opts) > 0 {
		// Per-call options may add headers; clone so they never mutate
		// the base client's header map. With no options the shared map is
		// only read, so the clone is skipped.
		cfg.headers = c.cfg.headers.Clone()
		for _, opt := range opts {
			opt(&cfg)
		}
	}
	// Path-bound fields are excluded from query conversion below; the
	// substitution returns them so the template is walked only once.
	path, bound, err := runtime.SubstitutePathParamsBound(path, req)
	if err != nil {
		return err
	}

	// Encode the request body once. Retries re-send the same bytes, so the
	// encoding cost is paid a single time.
	var bodyBytes []byte
	switch body {
	case "*":
		data, err := cfg.codec.Marshal(req)
		if err != nil {
			return status.NewErrorf(codes.CodeInternal, "encode request: %v", err)
		}
		bodyBytes = data
	case "":
		if req != nil {
			query, err := runtime.MessageToQuery(req, bound...)
			if err != nil {
				return err
			}
			if len(query) > 0 {
				path += "?" + query.Encode()
			}
		}
	default:
		field, err := runtime.ExtractFieldValue(req, body)
		if err != nil {
			return err
		}
		data, err := cfg.codec.Marshal(field)
		if err != nil {
			return status.NewErrorf(codes.CodeInternal, "encode request: %v", err)
		}
		bodyBytes = data
	}

	// Apply the per-call deadline: set the header (so the server honors it)
	// and the local context deadline (so the client cancels).
	ctx, cancel := cfg.applyDeadline(ctx)
	defer cancel()

	// Build the invoker chain: interceptors wrap the HTTP round trip, and
	// the retry policy wraps the chain (interceptors observe each attempt).
	invoker := c.buildInvoker(&cfg)

	_, err = invoker(ctx, method, path, bodyBytes, req, resp, &cfg)
	return err
}

// buildInvoker composes the unary interceptor chain and the retry policy
// around the raw HTTP round trip.
func (c *ClientBase) buildInvoker(cfg *clientConfig) UnaryInvoker {
	chained := ChainUnaryInterceptors(cfg.unaryInterceptors, c.invokeUnary)
	if cfg.retryPolicy != nil {
		chained = retryInvoker(cfg.retryPolicy, chained)
	}
	return chained
}

// invokeUnary performs the raw HTTP round trip for one attempt. It returns
// the response body byte count and an RPC error. The request body is passed
// as bytes so retries can re-send without re-encoding.
func (c *ClientBase) invokeUnary(ctx context.Context, method, path string, body []byte, req, resp any, cfg *clientConfig) (int64, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return 0, err
	}
	for k, vs := range cfg.headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	if bodyReader != nil {
		httpReq.Header.Set(metadata.HeaderContentType, cfg.codec.Name())
	}
	httpReq.Header.Set(metadata.HeaderAccept, cfg.codec.Name())

	httpResp, err := cfg.httpClient.Do(httpReq)
	if err != nil {
		// Surface context expiry as an RPC status error, mirroring
		// grpc-go: a deadline produces DeadlineExceeded, a cancel
		// produces Canceled.
		if ctx.Err() == context.DeadlineExceeded {
			return 0, status.NewErrorf(codes.CodeDeadlineExceeded, "context deadline exceeded")
		}
		if ctx.Err() == context.Canceled {
			return 0, status.NewErrorf(codes.CodeCanceled, "context canceled")
		}
		return 0, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return 0, status.FromHTTP(httpResp)
	}

	// Read the response body once and decode from bytes. A unary response
	// body is the whole message; codec.Unmarshal (json.Unmarshal) avoids
	// the streaming decoder's per-call state allocations. The read buffer
	// is pooled: it is framework-owned and dead after Unmarshal.
	buf := clientReadBufPool.Get()
	defer clientReadBufPool.Put(buf)
	if _, err := buf.ReadFrom(httpResp.Body); err != nil {
		return 0, status.NewErrorf(codes.CodeInternal, "read response: %v", err)
	}
	if err := cfg.codec.Unmarshal(buf.Bytes(), resp); err != nil {
		return 0, status.NewErrorf(codes.CodeInternal, "decode response: %v", err)
	}
	return int64(buf.Len()), nil
}