package kernel

import (
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/runtime"
)

// OperationOption configures an Operation.
type OperationOption func(*Operation)

// WithOperationCodec sets the codec for an operation. Defaults to JSONCodec.
func WithOperationCodec(codec encoding.Codec) OperationOption {
	return func(e *Operation) { e.codec = codec }
}

// WithOperationUnaryInterceptors adds unary interceptors to an operation.
// The first interceptor is the outermost.
func WithOperationUnaryInterceptors(interceptors ...middleware.UnaryInterceptor) OperationOption {
	return func(e *Operation) { e.interceptors = append(e.interceptors, interceptors...) }
}

// WithBody sets the body mapping for an operation:
//
//	"*"     - the entire request body decodes into the request message
//	"field" - only the named field is decoded from the body
//	""      - no body; all unbound fields come from query params
func WithBody(body string) OperationOption {
	return func(e *Operation) { e.Body = body }
}

// WithResponseBody sets a field of the response message to use as the
// response body. Empty means the whole response message.
func WithResponseBody(field string) OperationOption {
	return func(e *Operation) { e.ResponseBody = field }
}

// WithRPC sets the RPC method name, used to derive the fully-qualified
// procedure name (e.g. "/acme.users.v1.UsersService/GetUser") when the
// operation is registered on a service.
func WithRPC(name string) OperationOption {
	return func(e *Operation) { e.RPC = name }
}

// WithRequestBuilder replaces the default reflection-based request
// building with a custom builder. Used by generated code for type-safe
// path parameter extraction.
func WithRequestBuilder(b RequestBuilder) OperationOption {
	return func(e *Operation) { e.requestBuilder = b }
}

// WithStreamType sets the streaming mode of the operation. It is set
// automatically by the NewServerStreamOperation, NewClientStreamOperation,
// and NewBidiStreamOperation constructors.
func WithStreamType(t runtime.StreamType) OperationOption {
	return func(e *Operation) { e.streamType = t }
}
