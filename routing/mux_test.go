package routing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/kernel"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/peer"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

type testUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type testGetReq struct {
	ID string `json:"id"`
}

type testCreateReq struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type testListReq struct {
	PageSize int `json:"page_size"`
}

type testListResp struct {
	Users []*testUser `json:"users"`
}

func testService() *kernel.Service {
	return kernel.NewService("test.v1.TestService").
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/users/{id}",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				if req.Msg().ID == "missing" {
					return nil, status.NewErrorf(codes.CodeNotFound, "user %q not found", req.Msg().ID)
				}
				return runtime.NewResponse(&testUser{ID: req.Msg().ID, Name: "Zara", Email: "zara@example.com"}), nil
			},
			kernel.WithRPC("GetUser"),
		)).
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/users",
			func(ctx runtime.Ctx, req *runtime.Request[testListReq]) (*runtime.Response[testListResp], error) {
				return runtime.NewResponse(&testListResp{Users: []*testUser{{ID: "1", Name: "A"}}}), nil
			},
			kernel.WithRPC("ListUsers"),
		)).
		Add(kernel.NewOperation(
			http.MethodPost,
			"/v1/users",
			func(ctx runtime.Ctx, req *runtime.Request[testCreateReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: "new", Name: req.Msg().Name, Email: req.Msg().Email}), nil
			},
			kernel.WithBody("*"),
			kernel.WithRPC("CreateUser"),
		))
}

func newTestMux(t *testing.T) *Mux {
	t.Helper()
	mux := NewMux()
	if err := mux.Register(testService()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return mux
}

func doRequest(t *testing.T, mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestMuxGetWithPathParam(t *testing.T) {
	mux := newTestMux(t)
	rec := doRequest(t, mux, http.MethodGet, "/v1/users/42", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got testUser
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != "42" {
		t.Errorf("ID = %q, want 42", got.ID)
	}
	if got.Name != "Zara" {
		t.Errorf("Name = %q, want Zara", got.Name)
	}
}

func TestMuxGetWithQueryParam(t *testing.T) {
	mux := newTestMux(t)
	rec := doRequest(t, mux, http.MethodGet, "/v1/users?page_size=10", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got testListResp
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Users) != 1 {
		t.Errorf("Users = %d, want 1", len(got.Users))
	}
}

func TestMuxPostWithBody(t *testing.T) {
	mux := newTestMux(t)
	rec := doRequest(t, mux, http.MethodPost, "/v1/users", `{"name":"Budi","email":"budi@example.com"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got testUser
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "Budi" || got.Email != "budi@example.com" {
		t.Errorf("got = %+v, want name=Budi email=budi@example.com", got)
	}
}

func TestMuxNotFound(t *testing.T) {
	mux := newTestMux(t)
	rec := doRequest(t, mux, http.MethodGet, "/v1/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestMuxMethodNotAllowed(t *testing.T) {
	mux := newTestMux(t)
	rec := doRequest(t, mux, http.MethodDelete, "/v1/users/42", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Errorf("Allow = %q, want to contain GET", allow)
	}
}

func TestMuxErrorMapping(t *testing.T) {
	mux := newTestMux(t)
	rec := doRequest(t, mux, http.MethodGet, "/v1/users/missing", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["code"] != "not_found" {
		t.Errorf("code = %v, want not_found", body["code"])
	}
	if !strings.Contains(body["message"].(string), "missing") {
		t.Errorf("message = %v, want to mention missing", body["message"])
	}
}

func TestMuxInterceptors(t *testing.T) {
	var order []string
	interceptor := middleware.UnaryInterceptorFunc(func(next middleware.UnaryFunc) middleware.UnaryFunc {
		return func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
			order = append(order, "before")
			resp, err := next(ctx, req)
			order = append(order, "after")
			return resp, err
		}
	})

	mux := NewMux(WithMuxUnaryInterceptors(interceptor))
	if err := mux.Register(testService()); err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, mux, http.MethodGet, "/v1/users/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(order) != 2 || order[0] != "before" || order[1] != "after" {
		t.Errorf("order = %v, want [before after]", order)
	}
}

func TestMuxInterceptorSeesSpec(t *testing.T) {
	var gotSpec runtime.Spec
	interceptor := middleware.UnaryInterceptorFunc(func(next middleware.UnaryFunc) middleware.UnaryFunc {
		return func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
			gotSpec = req.Spec()
			return next(ctx, req)
		}
	})

	mux := NewMux(WithMuxUnaryInterceptors(interceptor))
	if err := mux.Register(testService()); err != nil {
		t.Fatal(err)
	}
	doRequest(t, mux, http.MethodGet, "/v1/users/1", "")
	if gotSpec.Procedure != "/test.v1.TestService/GetUser" {
		t.Errorf("Procedure = %q, want /test.v1.TestService/GetUser", gotSpec.Procedure)
	}
}

func TestMuxServicePrefix(t *testing.T) {
	svc := testService()
	svc.Prefix = "/api"
	mux := NewMux()
	if err := mux.Register(svc); err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, mux, http.MethodGet, "/api/v1/users/42", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestMuxWrappedErrorMapping(t *testing.T) {
	svc := kernel.NewService("test.v1.TestService").
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/wrapped",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				base := status.NewErrorf(codes.CodeNotFound, "user %q not found", "42")
				base.Meta().Set("X-Error-Detail", "wrapped")
				return nil, fmt.Errorf("handler failed: %w", base)
			},
		)).
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/plain",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				return nil, fmt.Errorf("boom")
			},
		))

	mux := NewMux()
	if err := mux.Register(svc); err != nil {
		t.Fatal(err)
	}

	// Wrapped error: code + metadata must survive the wrap via errors.As.
	rec := doRequest(t, mux, http.MethodGet, "/v1/wrapped", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["code"] != "not_found" {
		t.Errorf("code = %v, want not_found", body["code"])
	}
	if got := rec.Header().Get("X-Error-Detail"); got != "wrapped" {
		t.Errorf("X-Error-Detail = %q, want wrapped (metadata must survive errors.As)", got)
	}

	// Plain error: Unknown mapping.
	rec = doRequest(t, mux, http.MethodGet, "/v1/plain", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["code"] != "unknown" {
		t.Errorf("code = %v, want unknown", body["code"])
	}
}

func TestMuxExactRouteNilParams(t *testing.T) {
	// Exact routes must not allocate a path-params map: the mux passes nil
	// (Pattern.Match allocates lazily, only when a {param} segment exists).
	var exactParams, wildcardParams map[string]string
	interceptor := middleware.UnaryInterceptorFunc(func(next middleware.UnaryFunc) middleware.UnaryFunc {
		return func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
			switch req.Spec().Procedure {
			case "/test.v1.TestService/Exact":
				exactParams = ctx.Params()
			case "/test.v1.TestService/GetUser":
				wildcardParams = ctx.Params()
			}
			return next(ctx, req)
		}
	})

	svc := kernel.NewService("test.v1.TestService").
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/exact",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: "exact"}), nil
			},
			kernel.WithRPC("Exact"),
		)).
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/users/{id}",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: req.Msg().ID}), nil
			},
			kernel.WithRPC("GetUser"),
		))

	mux := NewMux(WithMuxUnaryInterceptors(interceptor))
	if err := mux.Register(svc); err != nil {
		t.Fatal(err)
	}

	if rec := doRequest(t, mux, http.MethodGet, "/v1/exact", ""); rec.Code != http.StatusOK {
		t.Fatalf("exact status = %d, want 200", rec.Code)
	}
	if exactParams != nil {
		t.Errorf("exact route params = %v, want nil (no map allocation)", exactParams)
	}

	if rec := doRequest(t, mux, http.MethodGet, "/v1/users/42", ""); rec.Code != http.StatusOK {
		t.Fatalf("wildcard status = %d, want 200", rec.Code)
	}
	if wildcardParams == nil || wildcardParams["id"] != "42" {
		t.Errorf("wildcard route params = %v, want {id:42}", wildcardParams)
	}
}

func TestMuxSpecificity(t *testing.T) {
	// /v1/users/me must win over /v1/users/{id}.
	svc := kernel.NewService("test.v1.TestService").
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/users/{id}",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: "param:" + req.Msg().ID}), nil
			},
		)).
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/users/me",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: "me"}), nil
			},
		))

	mux := NewMux()
	if err := mux.Register(svc); err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, mux, http.MethodGet, "/v1/users/me", "")
	var got testUser
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "me" {
		t.Errorf("ID = %q, want me (specific route should win)", got.ID)
	}
}

func mustAny(t *testing.T, s string) *anypb.Any {
	t.Helper()
	d, err := anypb.New(structpb.NewStringValue(s))
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return d
}

func TestServerHonorsClientDeadline(t *testing.T) {
	mux := NewMux()
	svc := kernel.NewService("test.v1.TestService").Add(kernel.NewOperation(
		http.MethodGet,
		"/v1/slow",
		func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
			select {
			case <-ctx.Done():
				return nil, status.NewErrorf(codes.CodeDeadlineExceeded, "deadline exceeded")
			case <-time.After(2 * time.Second):
				return runtime.NewResponse(&testUser{ID: "1"}), nil
			}
		},
		kernel.WithRPC("Slow"),
	))
	if err := mux.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/slow", nil)
	req.Header.Set(metadata.HeaderGrpcTimeout, "50m")
	rec := httptest.NewRecorder()
	start := time.Now()
	mux.ServeHTTP(rec, req)
	if time.Since(start) > time.Second {
		t.Fatalf("request took %v, want fast deadline expiry", time.Since(start))
	}
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 (deadline exceeded)", rec.Code)
	}
}

func TestErrorDetailsOnWire(t *testing.T) {
	mux := NewMux()
	detail := mustAny(t, "quota exceeded")
	svc := kernel.NewService("test.v1.TestService").Add(kernel.NewOperation(
		http.MethodGet,
		"/v1/quota",
		func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
			return nil, status.NewErrorf(codes.CodeResourceExhausted, "out of quota").WithDetails(detail)
		},
		kernel.WithRPC("Quota"),
	))
	if err := mux.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/quota", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rec.Header().Get(metadata.HeaderGrpcStatusDetails) == "" {
		t.Fatal("grpc-status-details-bin header missing")
	}
}

// --- Benchmarks ---

func benchMux() *Mux {
	svc := kernel.NewService("test.v1.TestService").
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/users/{id}",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: req.Msg().ID, Name: "Zara", Email: "zara@example.com"}), nil
			},
			kernel.WithRPC("GetUser"),
		)).
		Add(kernel.NewOperation(
			http.MethodPost,
			"/v1/users",
			func(ctx runtime.Ctx, req *runtime.Request[testCreateReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: "new", Name: req.Msg().Name, Email: req.Msg().Email}), nil
			},
			kernel.WithBody("*"),
			kernel.WithRPC("CreateUser"),
		))
	mux := NewMux()
	if err := mux.Register(svc); err != nil {
		panic(err)
	}
	return mux
}

func BenchmarkMuxGet(b *testing.B) {
	mux := benchMux()
	req := httptest.NewRequest(http.MethodGet, "/v1/users/42?page_size=10", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}
}

func BenchmarkMuxPost(b *testing.B) {
	mux := benchMux()
	body := `{"name":"Budi","email":"budi@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader(body))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		req.Body = io.NopCloser(strings.NewReader(body))
		mux.ServeHTTP(rec, req)
	}
}

func benchMuxGenerated() *Mux {
	svc := kernel.NewService("test.v1.TestService").
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/users/{id}",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: req.Msg().ID, Name: "Zara", Email: "zara@example.com"}), nil
			},
			kernel.WithRPC("GetUser"),
			kernel.WithRequestBuilder(func(ctx runtime.Ctx, r *http.Request, params map[string]string, spec runtime.Spec, codec encoding.Codec) (runtime.AnyRequest, error) {
				msg := &testGetReq{}
				meta := ctx.Meta()
				if v, ok := params["id"]; ok {
					msg.ID = v
				}
				return runtime.NewRequestWithMeta(msg, meta.Header, spec, peer.Peer{Addr: r.RemoteAddr, Protocol: r.Proto}), nil
			}),
		))
	mux := NewMux()
	if err := mux.Register(svc); err != nil {
		panic(err)
	}
	return mux
}

func BenchmarkMuxGetGenerated(b *testing.B) {
	mux := benchMuxGenerated()
	req := httptest.NewRequest(http.MethodGet, "/v1/users/42?page_size=10", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}
}

func benchMuxMany(n int) *Mux {
	svc := kernel.NewService("test.v1.TestService")
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("user%d", i)
		svc = svc.Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/users/"+id,
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: req.Msg().ID}), nil
			},
			kernel.WithRPC("GetUser"+id),
		))
	}
	svc = svc.Add(kernel.NewOperation(
		http.MethodGet,
		"/v1/users/{id}",
		func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
			return runtime.NewResponse(&testUser{ID: req.Msg().ID}), nil
		},
		kernel.WithRPC("GetUserAny"),
	))
	mux := NewMux()
	if err := mux.Register(svc); err != nil {
		panic(err)
	}
	return mux
}

func BenchmarkMuxExactManyRoutes(b *testing.B) {
	mux := benchMuxMany(100)
	req := httptest.NewRequest(http.MethodGet, "/v1/users/user99", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}
}

func benchMuxTreeSharedPrefix(n int) *Mux {
	svc := kernel.NewService("test.v1.TestService")
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("r%d", i)
		svc = svc.Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/"+id+"/users/{id}",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: req.Msg().ID}), nil
			},
			kernel.WithRPC("GetUser"+id),
		))
	}
	mux := NewMux()
	if err := mux.Register(svc); err != nil {
		panic(err)
	}
	return mux
}

func benchMuxTreeDistinctPrefix(n int) *Mux {
	svc := kernel.NewService("test.v1.TestService")
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("r%d", i)
		svc = svc.Add(kernel.NewOperation(
			http.MethodGet,
			"/"+id+"/users/{id}",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
				return runtime.NewResponse(&testUser{ID: req.Msg().ID}), nil
			},
			kernel.WithRPC("GetUser"+id),
		))
	}
	mux := NewMux()
	if err := mux.Register(svc); err != nil {
		panic(err)
	}
	return mux
}

func benchTreeSharedPrefix(b *testing.B, n int) {
	mux := benchMuxTreeSharedPrefix(n)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/r%d/users/42", n-1), nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}
}

func benchTreeDistinctPrefix(b *testing.B, n int) {
	mux := benchMuxTreeDistinctPrefix(n)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/r%d/users/42", n-1), nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}
}

func BenchmarkMuxTreeSharedPrefix1(b *testing.B)    { benchTreeSharedPrefix(b, 1) }
func BenchmarkMuxTreeSharedPrefix10(b *testing.B)   { benchTreeSharedPrefix(b, 10) }
func BenchmarkMuxTreeSharedPrefix100(b *testing.B)  { benchTreeSharedPrefix(b, 100) }
func BenchmarkMuxTreeSharedPrefix1000(b *testing.B) { benchTreeSharedPrefix(b, 1000) }

func BenchmarkMuxTreeDistinctPrefix1(b *testing.B)    { benchTreeDistinctPrefix(b, 1) }
func BenchmarkMuxTreeDistinctPrefix10(b *testing.B)   { benchTreeDistinctPrefix(b, 10) }
func BenchmarkMuxTreeDistinctPrefix100(b *testing.B)  { benchTreeDistinctPrefix(b, 100) }
func BenchmarkMuxTreeDistinctPrefix1000(b *testing.B) { benchTreeDistinctPrefix(b, 1000) }

func benchPostSize(b *testing.B, size int) {
	mux := benchMux()
	body := `{"name":"` + strings.Repeat("x", size) + `","email":"b@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader(body))
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		req.Body = io.NopCloser(strings.NewReader(body))
		mux.ServeHTTP(rec, req)
	}
}

func BenchmarkMuxPost1KB(b *testing.B)  { benchPostSize(b, 1<<10) }
func BenchmarkMuxPost64KB(b *testing.B) { benchPostSize(b, 64<<10) }
func BenchmarkMuxPost1MB(b *testing.B)  { benchPostSize(b, 1<<20) }

func BenchmarkMuxGetParallel(b *testing.B) {
	mux := benchMux()
	req := httptest.NewRequest(http.MethodGet, "/v1/users/42?page_size=10", nil)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
		}
	})
}

func benchMuxProto() *Mux {
	svc := kernel.NewService("test.v1.TestService").
		Add(kernel.NewOperation(
			http.MethodGet,
			"/v1/proto",
			func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[descriptorpb.FileDescriptorProto], error) {
				return runtime.NewResponse(&descriptorpb.FileDescriptorProto{
					Name:    proto.String("users/v1/users.proto"),
					Package: proto.String("acme.users.v1"),
				}), nil
			},
		))
	mux := NewMux()
	if err := mux.Register(svc); err != nil {
		panic(err)
	}
	return mux
}

func BenchmarkMuxGetProto(b *testing.B) {
	mux := benchMuxProto()
	req := httptest.NewRequest(http.MethodGet, "/v1/proto", nil)
	req.Header.Set(metadata.HeaderAccept, metadata.ContentTypeProtobuf)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}
}
