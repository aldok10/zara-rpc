package kernel

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aldok10/zara-rpc/metadata"
)

func TestDeadlineFromRequest(t *testing.T) {
	t.Run("grpc-timeout wins over grpc-deadline", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/v1/users/1", nil)
		r.Header.Set(metadata.HeaderGrpcTimeout, "10S")
		r.Header.Set(metadata.HeaderGrpcDeadline, "2030-01-02T03:04:05Z")
		d, ok := deadlineFromRequest(r)
		if !ok || d != 10*time.Second {
			t.Fatalf("deadlineFromRequest = (%v, %v), want (10s, true)", d, ok)
		}
	})

	t.Run("grpc-deadline used when no timeout", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/v1/users/1", nil)
		r.Header.Set(metadata.HeaderGrpcDeadline, time.Now().Add(time.Hour).Format(time.RFC3339))
		d, ok := deadlineFromRequest(r)
		if !ok || d <= 0 || d > time.Hour {
			t.Fatalf("deadlineFromRequest = (%v, %v), want (~1h, true)", d, ok)
		}
	})

	t.Run("malformed ignored", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/v1/users/1", nil)
		r.Header.Set(metadata.HeaderGrpcTimeout, "not-a-timeout")
		if _, ok := deadlineFromRequest(r); ok {
			t.Fatal("malformed timeout must be ignored")
		}
	})

	t.Run("past deadline expires immediately", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/v1/users/1", nil)
		r.Header.Set(metadata.HeaderGrpcDeadline, time.Now().Add(-time.Minute).Format(time.RFC3339))
		d, ok := deadlineFromRequest(r)
		if !ok || d != 0 {
			t.Fatalf("deadlineFromRequest = (%v, %v), want (0, true)", d, ok)
		}
	})
}