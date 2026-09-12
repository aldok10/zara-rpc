package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/aldok10/zara-rpc/metadata"
)

// TestSSECloseStopsBackgroundGoroutine is a regression test for the SSE
// body-close goroutine. The goroutine used to wait only on ctx.Done(); a
// caller that closed the stream without canceling the context leaked the
// goroutine forever. Close() now signals the goroutine directly.
func TestSSECloseStopsBackgroundGoroutine(t *testing.T) {
	defer goleak.VerifyNone(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeEventStream)
		w.WriteHeader(http.StatusOK)
		// Stream forever; the client's Close() must tear this down.
		for {
			if _, err := w.Write([]byte("data: {}\n\n")); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(time.Millisecond)
		}
	}))
	defer srv.Close()

	// context.Background() is deliberate: the old implementation leaked
	// when Close() was called without canceling the context.
	client := NewClientBase(srv.URL)
	stream, err := client.DoServerStream(
		context.Background(),
		http.MethodGet,
		"/stream",
		nil,
		reflect.TypeOf(struct{}{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Receive(); err != nil {
		t.Fatalf("receive: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}