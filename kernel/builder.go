// OperationBuilder: zero-allocation in-place operation construction.
package kernel

import (
	"net/http"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/middleware"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

// OperationBuilder builds an Operation in place with zero extra
// allocations. It is the underlying type of Operation, so converting a
// *Operation to *OperationBuilder is free (identical underlying types);
// setters mutate the struct and return the builder for chaining. The
// builder is generic because Go methods cannot declare their own type
// parameters, and the handler setters need Req/Res.
//
//	var op Operation
//	(*OperationBuilder[Req, Res])(&op).
//		SetMethod("GET").
//		SetPath("/v1/users/{id}").
//		SetUnaryHandler(handler).
//		Build()
type OperationBuilder[Req, Res any] Operation

// SetMethod sets the HTTP method.
func (b *OperationBuilder[Req, Res]) SetMethod(method string) *OperationBuilder[Req, Res] {
	b.Method = method
	return b
}

// SetPath sets the path template, e.g. "/v1/users/{id}".
func (b *OperationBuilder[Req, Res]) SetPath(path string) *OperationBuilder[Req, Res] {
	b.Path = path
	return b
}

// SetProcedure sets the fully-qualified procedure name. When empty, it is
// derived from the service name and RPC on registration.
func (b *OperationBuilder[Req, Res]) SetProcedure(procedure string) *OperationBuilder[Req, Res] {
	b.Procedure = procedure
	return b
}

// SetRPC sets the RPC method name, used to derive the procedure name.
func (b *OperationBuilder[Req, Res]) SetRPC(rpc string) *OperationBuilder[Req, Res] {
	b.RPC = rpc
	return b
}

// SetBody sets the body mapping:
//
//	"*"     - the entire request body decodes into the request message
//	"field" - only the named field is decoded from the body
//	""      - no body; all unbound fields come from query params
func (b *OperationBuilder[Req, Res]) SetBody(body string) *OperationBuilder[Req, Res] {
	b.Body = body
	return b
}

// SetResponseBody sets a field of the response message to use as the
// response body. Empty means the whole response message.
func (b *OperationBuilder[Req, Res]) SetResponseBody(field string) *OperationBuilder[Req, Res] {
	b.ResponseBody = field
	return b
}

// SetCodec sets the codec. Defaults to JSONCodec when unset.
func (b *OperationBuilder[Req, Res]) SetCodec(codec encoding.Codec) *OperationBuilder[Req, Res] {
	b.codec = codec
	return b
}

// SetInterceptors appends unary interceptors. The first is the outermost.
func (b *OperationBuilder[Req, Res]) SetInterceptors(interceptors ...middleware.UnaryInterceptor) *OperationBuilder[Req, Res] {
	b.interceptors = append(b.interceptors, interceptors...)
	return b
}

// SetStreamInterceptors appends per-message stream interceptors.
func (b *OperationBuilder[Req, Res]) SetStreamInterceptors(interceptors ...middleware.StreamInterceptor) *OperationBuilder[Req, Res] {
	b.streamInterceptors = append(b.streamInterceptors, interceptors...)
	return b
}

// SetRequestBuilder replaces the default reflection-based request building
// with a custom builder. Used by generated code for type-safe path
// parameter extraction.
func (b *OperationBuilder[Req, Res]) SetRequestBuilder(rb RequestBuilder) *OperationBuilder[Req, Res] {
	b.requestBuilder = rb
	return b
}

// SetStreamType sets the streaming mode. The handler setters set it
// automatically; use this only for custom modes.
func (b *OperationBuilder[Req, Res]) SetStreamType(t runtime.StreamType) *OperationBuilder[Req, Res] {
	b.streamType = t
	return b
}

// SetUnaryHandler installs a unary handler and captures the request and
// response types.
func (b *OperationBuilder[Req, Res]) SetUnaryHandler(handler func(runtime.Ctx, *runtime.Request[Req]) (*runtime.Response[Res], error)) *OperationBuilder[Req, Res] {
	b.reqType = elemType[Req]()
	b.resType = elemType[Res]()
	b.unary = func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
		typedReq, ok := req.(*runtime.Request[Req])
		if !ok {
			return nil, status.NewErrorf(codes.CodeInternal, "unexpected request type %T", req)
		}
		return handler(ctx, typedReq)
	}
	b.newRequest = func(ctx runtime.Ctx, r *http.Request, params map[string]string, codec encoding.Codec) (runtime.AnyRequest, error) {
		meta := ctx.Meta()
		op := (*Operation)(b)
		if op.requestBuilder != nil {
			req, err := op.requestBuilder(ctx, r, params, op.spec(), codec)
			if err != nil {
				return nil, err
			}
			if typed, ok := req.(*runtime.Request[Req]); ok {
				typed.WithRequestMeta(meta)
			}
			return req, nil
		}
		msg := new(Req)
		if err := runtime.PopulateMessage(msg, params, meta.Query, meta.Body, op.Body, codec); err != nil {
			return nil, err
		}
		return runtime.NewRequestWithMeta(msg, meta.Header, op.spec(), ctx.Peer()).WithRequestMeta(meta), nil
	}
	return b
}

// SetServerStreamHandler installs a server-streaming handler. Each
// response is written as an SSE event.
func (b *OperationBuilder[Req, Res]) SetServerStreamHandler(handler ServerStreamFunc[Req, Res]) *OperationBuilder[Req, Res] {
	b.reqType = elemType[Req]()
	b.resType = elemType[Res]()
	b.streamType = runtime.StreamTypeServer
	b.serverStream = func(ctx runtime.Ctx, req runtime.AnyRequest, stream runtime.ServerStream[any]) error {
		typedReq, ok := req.(*runtime.Request[Req])
		if !ok {
			// WebSocket transports build requests without an HTTP request;
			// reconstruct the typed request from the message.
			msg, ok := req.Any().(*Req)
			if !ok {
				return status.NewErrorf(codes.CodeInternal, "unexpected request type %T", req)
			}
			typedReq = runtime.NewRequestWithMeta(msg, req.Header(), req.Spec(), req.Peer())
		}
		return handler(ctx, typedReq, &typedServerStream[Res]{inner: stream})
	}
	b.newRequest = func(ctx runtime.Ctx, r *http.Request, params map[string]string, codec encoding.Codec) (runtime.AnyRequest, error) {
		meta := ctx.Meta()
		op := (*Operation)(b)
		if op.requestBuilder != nil {
			req, err := op.requestBuilder(ctx, r, params, op.spec(), codec)
			if err != nil {
				return nil, err
			}
			if typed, ok := req.(*runtime.Request[Req]); ok {
				typed.WithRequestMeta(meta)
			}
			return req, nil
		}
		msg := new(Req)
		if err := runtime.PopulateMessage(msg, params, meta.Query, meta.Body, op.Body, codec); err != nil {
			return nil, err
		}
		return runtime.NewRequestWithMeta(msg, meta.Header, op.spec(), ctx.Peer()).WithRequestMeta(meta), nil
	}
	return b
}

// SetClientStreamHandler installs a client-streaming handler. Requests are
// read from the stream as newline-delimited JSON.
func (b *OperationBuilder[Req, Res]) SetClientStreamHandler(handler ClientStreamFunc[Req, Res]) *OperationBuilder[Req, Res] {
	b.reqType = elemType[Req]()
	b.resType = elemType[Res]()
	b.streamType = runtime.StreamTypeClient
	b.clientStream = func(ctx runtime.Ctx, stream runtime.ClientStream[any]) (runtime.AnyResponse, error) {
		return handler(ctx, &typedClientStream[Req]{inner: stream})
	}
	return b
}

// SetBidiStreamHandler installs a bidi-streaming handler, transported over
// a WebSocket.
func (b *OperationBuilder[Req, Res]) SetBidiStreamHandler(handler BidiStreamFunc[Req, Res]) *OperationBuilder[Req, Res] {
	b.reqType = elemType[Req]()
	b.resType = elemType[Res]()
	b.streamType = runtime.StreamTypeBidi
	b.bidiStream = func(ctx runtime.Ctx, stream runtime.BidiStream[any, any]) error {
		return handler(ctx, &typedBidiStream[Req, Res]{inner: stream})
	}
	return b
}

// Build finalizes the Operation: defaults the codec and computes the cached
// Spec. It returns the Operation pointer; the struct escapes to the heap
// exactly once. Path-pattern validation happens at registration time
// (routing.Mux.Register) or handler construction (kernel.NewHandler), which
// panic or return an error on an invalid template.
func (b *OperationBuilder[Req, Res]) Build() *Operation {
	op := (*Operation)(b)
	if op.codec == nil {
		op.codec = encoding.JSONCodec{}
	}
	op.specCache = computeSpec(op)
	return op
}
