package client

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain verifies that no goroutines leak after the client tests run.
// The client spawns background goroutines for streaming transports (SSE
// body close, NDJSON response pump); a leak here means a stream was not
// closed or a context was not canceled.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}