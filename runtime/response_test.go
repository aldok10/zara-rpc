package runtime

import (
	"testing"
)

func TestResponse(t *testing.T) {
	type dummy struct {
		Name string
	}

	resp := NewResponse(&dummy{Name: "Zara"})
	if resp.Msg().Name != "Zara" {
		t.Errorf("Msg().Name = %q, want Zara", resp.Msg().Name)
	}
	if resp.HeaderSet() {
		t.Error("HeaderSet() = true, want false before header access")
	}
	resp.Header().Set("X-Custom", "value")
	if !resp.HeaderSet() {
		t.Error("HeaderSet() = false, want true after header access")
	}
	if resp.Header().Get("X-Custom") != "value" {
		t.Errorf("Header X-Custom = %q, want value", resp.Header().Get("X-Custom"))
	}
	if resp.Any() == nil {
		t.Error("Any() returned nil")
	}
}