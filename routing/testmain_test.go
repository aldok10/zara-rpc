package routing

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain verifies that no goroutines leak after the test suite runs.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}