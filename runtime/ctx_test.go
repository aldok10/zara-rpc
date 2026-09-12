package runtime

import (
	"context"
	"testing"

	"github.com/aldok10/zara-rpc/metadata"
)

func TestCtxWithContext(t *testing.T) {
	ctx := context.Background()
	c := NewCtx(ctx, metadata.RequestMeta{}, nil)
	derived, cancel := context.WithCancel(ctx)
	defer cancel()
	c2 := c.WithContext(derived)
	if c2.Context() != derived {
		t.Fatal("WithContext did not replace the wrapped context")
	}
	// The shared state survives.
	if c2.state != c.state {
		t.Fatal("WithContext must preserve the shared state")
	}
}
