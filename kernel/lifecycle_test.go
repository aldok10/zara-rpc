package kernel

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestServerLifecycle(t *testing.T) {
	srv := New("127.0.0.1:0", http.NewServeMux())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 2*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("ListenAndServe error = %v, want http.ErrServerClosed or nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ListenAndServe did not exit after Shutdown")
	}
}