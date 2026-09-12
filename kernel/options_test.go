package kernel

import (
	"testing"

	"github.com/aldok10/zara-rpc/encoding"
)

func TestOperationOptions(t *testing.T) {
	op := &Operation{}
	WithBody("*")(op)
	if op.Body != "*" {
		t.Errorf("Body = %q, want *", op.Body)
	}

	WithResponseBody("data")(op)
	if op.ResponseBody != "data" {
		t.Errorf("ResponseBody = %q, want data", op.ResponseBody)
	}

	WithRPC("GetUser")(op)
	if op.RPC != "GetUser" {
		t.Errorf("RPC = %q, want GetUser", op.RPC)
	}

	WithOperationCodec(encoding.JSONCodec{})(op)
	if op.codec == nil {
		t.Error("codec is nil")
	}
}