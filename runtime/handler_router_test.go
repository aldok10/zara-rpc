package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aldok10/zara-rpc/metadata"
)

// tagHandler writes a marker so tests can tell which handler served a
// request.
func tagHandler(tag string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Served-By", tag)
		w.WriteHeader(http.StatusOK)
	})
}

func TestHandlerRouterGRPC(t *testing.T) {
	router := NewHandlerRouter(tagHandler("mux"), WithGRPC(tagHandler("grpc")))

	// HTTP/2 + application/grpc content-type → gRPC handler.
	req := httptest.NewRequest(http.MethodPost, "/acme.users.v1.UsersService/GetUser", nil)
	req.ProtoMajor = 2
	req.Header.Set(metadata.HeaderContentType, metadata.ContentTypeGRPC)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Served-By"); got != "grpc" {
		t.Fatalf("gRPC request served by %q; want grpc", got)
	}

	// HTTP/2 but wrong content-type → mux.
	req = httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	req.ProtoMajor = 2
	req.Header.Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Served-By"); got != "mux" {
		t.Fatalf("JSON request served by %q; want mux", got)
	}
}

func TestHandlerRouterMuxFallback(t *testing.T) {
	// No options: every request goes to the mux.
	router := NewHandlerRouter(tagHandler("mux"))

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.ProtoMajor = 2
	req.Header.Set(metadata.HeaderContentType, metadata.ContentTypeGRPC)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Served-By"); got != "mux" {
		t.Fatalf("option-less router served by %q; want mux", got)
	}
}