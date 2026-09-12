package kernel

import (
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/middleware"
)

// PrepareOperation applies mux-level defaults to an operation before it is
// registered: the default codec, the service-qualified procedure name, the
// prefixed path, and the mux-level interceptor chains. It refreshes the
// cached Spec so the hot path reads the final values. Called by
// routing.Mux.Register.
func PrepareOperation(e *Operation, serviceName string, codec encoding.Codec, unary []middleware.UnaryInterceptor, stream []middleware.StreamInterceptor, path string) {
	if e.codec == nil {
		e.codec = codec
	}
	if e.Procedure == "" {
		if serviceName != "" && e.RPC != "" {
			e.Procedure = "/" + serviceName + "/" + e.RPC
		} else if e.RPC != "" {
			e.Procedure = "/" + e.RPC
		}
	}
	e.Path = path
	e.specCache = computeSpec(e)

	// Apply mux-level interceptors before operation-level ones.
	e.interceptors = append(append([]middleware.UnaryInterceptor{}, unary...), e.interceptors...)
	// Stream interceptors are mux-level only; the operation has no
	// per-operation stream interceptor option today.
	e.streamInterceptors = append([]middleware.StreamInterceptor{}, stream...)
}
