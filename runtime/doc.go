// Package runtime is the zara-rpc framework core facade: routing, operations,
// HTTP adaptation, request/response types, path templates, message population,
// path converters, substitution, interceptors, and streams. It mirrors
// grpc-gateway's runtime package: the HTTP mux, typed operations, the HTTP
// handler, request and response types, streams, interceptors, and the
// request/response plumbing shared by the framework and generated code.
//
// The framework is connect-go-style typed handlers over pure net/http, with
// grpc-gateway-style declarative HTTP operations from google.api.http
// annotations. Cross-cutting concerns live in public subpackages that users
// import directly:
//
//	codes      — RPC error code enum (gRPC-compatible)
//	status     — RPC errors built on codes
//	encoding   — JSON, JSONV2, XML, Protobuf codecs + registry
//	metadata   — RequestMeta: headers, query, body, cookies in context
//	peer       — client peer info (addr, protocol) in context
//	client     — HTTP client core (DoUnary, DoServerStream, ...)
//	server     — HTTP server wrapper + streaming transports (SSE, NDJSON, WebSocket)
//
// The layout mirrors grpc-gateway: the framework core in runtime, the code
// generator at the repo root (protoc-gen-zararpc), and public subpackages
// for cross-cutting concerns, with functional options (WithXxx) for
// configuration.
//
// File-level responsibilities (the package is split by responsibility so an
// agent can navigate by name):
//
//	pattern.go      — path template parsing and matching (Pattern, ParsePattern)
//	pathconvert.go  — path parameter converters (PathString ... PathInt64Slice)
//	populate.go     — message population from path/query/body (PopulateMessage)
//	substitute.go   — client-side path substitution + query conversion
//	request.go      — Request[T] typed request wrapper (and Spec)
//	response.go     — Response[T] typed response wrapper
//	ctx.go          — Ctx handler context (transport metadata, codec, protocol, spec, peer)
//	context.go      — context helpers (WithPathParams, WithSpec)
//	mux.go          — route registry + dispatcher (Mux)
//	endpoint.go     — Operation type, options, constructors (RPC+HTTP binding)
//	builder.go      — OperationBuilder (zero-allocation in-place construction)
//	handler.go      — HTTP adapter (Handler)
//	service.go      — Service + ServiceBuilder (registration)
//	interceptor.go  — unary + stream interceptor types and chains
//	stream.go       — stream types (ServerStream, ClientStream, BidiStream)
//	version.go      — Version constant
package runtime
