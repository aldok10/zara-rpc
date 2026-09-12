package runtime

import (
	"net/http"
	"testing"

	"github.com/aldok10/zara-rpc/peer"
)

func TestRequest(t *testing.T) {
	type dummy struct {
		Name string
	}

	header := http.Header{"X-Test": []string{"val"}}
	spec := Spec{Procedure: "/test.v1.Test/Run", Path: "/v1/test"}
	p := peer.Peer{Addr: "127.0.0.1:8080", Protocol: "http"}

	req := NewRequestWithMeta(&dummy{Name: "Zara"}, header, spec, p)
	if req.Msg().Name != "Zara" {
		t.Errorf("Msg().Name = %q, want Zara", req.Msg().Name)
	}
	if req.Header().Get("X-Test") != "val" {
		t.Errorf("Header X-Test = %q, want val", req.Header().Get("X-Test"))
	}
	if req.Spec().Procedure != "/test.v1.Test/Run" {
		t.Errorf("Spec = %v, want procedure /test.v1.Test/Run", req.Spec())
	}
	if req.Peer().Addr != "127.0.0.1:8080" {
		t.Errorf("Peer = %v, want 127.0.0.1:8080", req.Peer())
	}
	if req.Any() == nil {
		t.Error("Any() returned nil")
	}
}