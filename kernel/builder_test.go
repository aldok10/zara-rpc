package kernel

import (
	"net/http"
	"testing"

	"github.com/aldok10/zara-rpc/runtime"
)

type testReq struct {
	ID string `json:"id"`
}

type testRes struct {
	Name string `json:"name"`
}

func TestOperationBuilder(t *testing.T) {
	var op Operation
	built := (*OperationBuilder[testReq, testRes])(&op).
		SetMethod(http.MethodGet).
		SetPath("/v1/users/{id}").
		SetRPC("GetUser").
		SetUnaryHandler(func(ctx runtime.Ctx, req *runtime.Request[testReq]) (*runtime.Response[testRes], error) {
			return runtime.NewResponse(&testRes{Name: "Zara"}), nil
		}).
		Build()

	if built == nil {
		t.Fatal("Build returned nil")
	}
	if built.Method != http.MethodGet {
		t.Errorf("Method = %q, want GET", built.Method)
	}
	if built.Path != "/v1/users/{id}" {
		t.Errorf("Path = %q, want /v1/users/{id}", built.Path)
	}
	if built.RPC != "GetUser" {
		t.Errorf("RPC = %q, want GetUser", built.RPC)
	}
	if built.unary == nil {
		t.Error("unary is nil")
	}
}

func BenchmarkOperationBuilder(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var op Operation
		(*OperationBuilder[testReq, testRes])(&op).
			SetMethod(http.MethodGet).
			SetPath("/v1/users/{id}").
			SetRPC("GetUser").
			SetUnaryHandler(func(ctx runtime.Ctx, req *runtime.Request[testReq]) (*runtime.Response[testRes], error) {
				return runtime.NewResponse(&testRes{Name: "Zara"}), nil
			}).
			Build()
	}
}

func BenchmarkNewOperation(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		NewOperation(
			http.MethodGet,
			"/v1/users/{id}",
			func(ctx runtime.Ctx, req *runtime.Request[testReq]) (*runtime.Response[testRes], error) {
				return runtime.NewResponse(&testRes{Name: "Zara"}), nil
			},
			WithRPC("GetUser"),
		)
	}
}