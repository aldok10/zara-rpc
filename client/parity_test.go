package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/status"
)

func TestClientInterceptorChain(t *testing.T) {
	var order []string
	interceptor := func(name string) UnaryInterceptor {
		return UnaryInterceptorFunc(func(next UnaryInvoker) UnaryInvoker {
			return func(ctx context.Context, method, path string, body []byte, req, resp any, cfg *clientConfig) (int64, error) {
				order = append(order, name+":before")
				n, err := next(ctx, method, path, body, req, resp, cfg)
				order = append(order, name+":after")
				return n, err
			}
		})
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{"id":"42","name":"Zara","email":"z@e.com"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL, WithUnaryInterceptors(
		interceptor("outer"),
		interceptor("inner"),
	))
	var got testUser
	if err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users/{id}", "", &testGetReq{ID: "42"}, &got); err != nil {
		t.Fatalf("call: %v", err)
	}

	want := []string{"outer:before", "inner:before", "inner:after", "outer:after"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestClientInterceptorShortCircuit(t *testing.T) {
	var hitServer atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitServer.Store(true)
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL, WithUnaryInterceptors(
		UnaryInterceptorFunc(func(next UnaryInvoker) UnaryInvoker {
			return func(ctx context.Context, method, path string, body []byte, req, resp any, cfg *clientConfig) (int64, error) {
				return 0, status.NewErrorf(codes.CodePermissionDenied, "blocked by interceptor")
			}
		}),
	))
	var got testUser
	err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users/{id}", "", &testGetReq{ID: "42"}, &got)
	if status.Code(err) != codes.CodePermissionDenied {
		t.Fatalf("code = %v, want permission_denied", status.Code(err))
	}
	if hitServer.Load() {
		t.Fatal("server must not be reached when an interceptor short-circuits")
	}
}

func TestClientRetryUnavailable(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"code":"unavailable","message":"try again"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL, WithRetryPolicy(RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
	}))
	var got testUser
	err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users/{id}", "", &testGetReq{ID: "42"}, &got)
	if status.Code(err) != codes.CodeUnavailable {
		t.Fatalf("code = %v, want unavailable", status.Code(err))
	}
	if attempts.Load() != 3 {
		t.Errorf("attempts = %d, want 3", attempts.Load())
	}
}

func TestClientRetryNonIdempotentNotRetried(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"code":"unavailable","message":"try again"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL, WithRetryPolicy(RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
	}))
	var got testUser
	// POST is not idempotent by default -> no retry.
	err := c.DoUnary(context.Background(), http.MethodPost, "/v1/users", "*", &testCreateReq{Name: "Budi"}, &got)
	if status.Code(err) != codes.CodeUnavailable {
		t.Fatalf("code = %v, want unavailable", status.Code(err))
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1 (no retry for non-idempotent)", attempts.Load())
	}
}

func TestClientRetryIdempotentFlag(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"code":"unavailable","message":"try again"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL, WithRetryPolicy(RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		Idempotent:     true, // POST retried anyway
	}))
	var got testUser
	err := c.DoUnary(context.Background(), http.MethodPost, "/v1/users", "*", &testCreateReq{Name: "Budi"}, &got)
	if status.Code(err) != codes.CodeUnavailable {
		t.Fatalf("code = %v, want unavailable", status.Code(err))
	}
	if attempts.Load() != 3 {
		t.Errorf("attempts = %d, want 3 (Idempotent flag forces retry)", attempts.Load())
	}
}

func TestClientRetryStopsOnSuccess(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"code":"unavailable","message":"try again"}`))
			return
		}
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{"id":"42","name":"Zara","email":"z@e.com"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL, WithRetryPolicy(RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
	}))
	var got testUser
	if err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users/{id}", "", &testGetReq{ID: "42"}, &got); err != nil {
		t.Fatalf("call: %v", err)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2 (stop after success)", attempts.Load())
	}
	if got.ID != "42" {
		t.Errorf("got = %+v, want id=42", got)
	}
}

func TestClientRetryContextCancelStopsBackoff(t *testing.T) {
	var attempts atomic.Int32
	// first signals that attempt 1 has been served, so cancel lands during
	// the backoff sleep. A fixed sleep here is racy under load: if the first
	// round trip outlives it, the cancel hits attempt 1 itself and the server
	// records zero attempts.
	first := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			defer close(first)
		}
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"code":"unavailable","message":"try again"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL, WithRetryPolicy(RetryPolicy{
		MaxAttempts:    5,
		InitialBackoff: time.Hour, // long backoff; cancel must interrupt it
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-first:
		case <-ctx.Done(): // the test finished early: do not leak this goroutine
			return
		}
		cancel()
	}()
	var got testUser
	start := time.Now()
	err := c.DoUnary(ctx, http.MethodGet, "/v1/users/{id}", "", &testGetReq{ID: "42"}, &got)
	if time.Since(start) > time.Second {
		t.Fatalf("call took %v, want fast cancel", time.Since(start))
	}
	if err == nil {
		t.Fatal("expected error after cancel")
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1 (cancel before second attempt)", attempts.Load())
	}
}

func TestClientTimeoutHeader(t *testing.T) {
	var gotTimeout string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTimeout = r.Header.Get(metadata.HeaderGrpcTimeout)
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{"id":"42","name":"Zara","email":"z@e.com"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	var user testUser
	if err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users/{id}", "", &testGetReq{ID: "42"}, &user, WithTimeout(2*time.Second)); err != nil {
		t.Fatalf("call: %v", err)
	}
	if gotTimeout == "" {
		t.Fatal("grpc-timeout header missing")
	}
}

func TestClientDeadlineCancels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep past the deadline; the client must cancel the request.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	var user testUser
	start := time.Now()
	err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users/{id}", "", &testGetReq{ID: "42"}, &user, WithTimeout(50*time.Millisecond))
	if time.Since(start) > time.Second {
		t.Fatalf("call took %v, want fast deadline expiry", time.Since(start))
	}
	if err == nil {
		t.Fatal("expected deadline error")
	}
	if status.Code(err) != codes.CodeDeadlineExceeded {
		t.Errorf("code = %v, want deadline_exceeded", status.Code(err))
	}
}