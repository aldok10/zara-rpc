# Architecture

Navigation map for the zara-rpc repository: entry points, dependency flow, where new features and tests go, and the key decisions that shape the code.

## Entry points

| Entry point | Location | What it does |
|---|---|---|
| Code generator | `protoc-gen-zararpc/` | protoc plugin: `.proto` -> `.pb.go` (with custom struct tags patched in) + `.zararpc.go` (handlers/clients/routes) + `.gateway.go` (REST -> gRPC) + `.grpc.go` (gRPC server adapter). A superset of protoc-gen-go; there is no separate protoc-gen-go or zararpc-tags step. |
| Example server | `examples/server` | Combined gRPC + HTTP demo server on :8080, JWT + RBAC auth demo, gRPC via the generated adapter + grpc-go `reflection.Register`. |
| Example gateway | `examples/gateway` | REST -> gRPC gateway on :8081. |
| Example client | `examples/client` | End-to-end client demo exercising JSON, Protobuf, SSE, NDJSON, and WebSocket transports. |

`examples/` is a git submodule (separate Go module, `replace => ../`); the example code lives in its own repository.

## Dependency flow

### Code generation: proto -> protoc-gen-zararpc -> kernel/routing/runtime -> handler

```text
users.proto (google.api.http annotations)
  -> protoc-gen-zararpc (buf generate)
       -> users.pb.go          message types + custom struct tags
       -> users.zararpc.go     Register_GetUser(...) *kernel.Operation,
                               RegisterUsersServiceRoutes(mux, svc),
                               UsersServiceHTTPClient
       -> users.gateway.go     REST -> gRPC gateway (calls metadata.HeaderPairsFromMeta,
                               status.FromGRPCStatus)
       -> users.grpc.go        RegisterUsersServiceGRPCServer(s, svc) via grpcbridge
  -> user code: implements the generated service handler interface
       -> routing.Mux routes the request to kernel.Handler
       -> handler calls the user function with runtime.Ctx
```

The generated `.zararpc.go` is the only consumer of the framework's public API surface: it builds `kernel.Operation` values, registers them on `routing.Mux` via `RegisterUsersServiceRoutes`, and constructs `UsersServiceHTTPClient` (zararpc client) for callers. The generated `.gateway.go` calls framework helpers (`metadata.HeaderPairsFromMeta`, `status.FromGRPCStatus`) instead of emitting per-package helpers. The generated `.grpc.go` imports `grpcbridge` (the only grpc-importing package in the root module) so the emitted adapter stays thin.

### Core DAG: runtime leaf -> kernel -> routing; runtime -> streaming -> transport

```text
HTTP / SSE / NDJSON / WebSocket (transport/, client/)
  -> routing.Mux (dispatch) -> kernel.Handler (HTTP adaptation) -> user handler
       -> kernel/    Operation, Service, lifecycle, typed stream adapters
       -> runtime/   Ctx, Request[T], Response[T], Spec, stream types,
                     PopulateMessage, SubstitutePathParams, path converters
       -> streaming/ stream adapters + transport registry
       -> transport/ sse/, ndjson/, websocket/ wire adapters
       -> middleware/ interceptor primitives + chains
       -> event/     stream event envelope
       -> codes/     RPC error code enum (leaf)
       -> status/    RPC error type built on codes
       -> encoding/  codecs (JSON, JSONV2, XML, Protobuf) + registry
       -> metadata/  RequestMeta + HTTP header constants
       -> peer/      client peer info
```

`runtime/` is the leaf of the core DAG: it imports only `codes`, `status`, `encoding`, `metadata`, `peer` — never `kernel`, `routing`, `streaming`, `transport`, `middleware`, or grpc. `kernel/` imports `runtime/`; `routing/` imports `kernel/` + `runtime/` + `streaming/` + `transport/`; `streaming/` imports `runtime/`; `transport/` imports `runtime/` + `streaming/`; `middleware/` imports `runtime/` + `kernel/`. `client/` imports `runtime/` + `middleware/` only — never `routing/` or `kernel/` (that would be a cycle).

## Where new features and tests go

| New feature | Location |
|---|---|
| Route matching / dispatch | `routing/` (`mux.go`, `pattern.go`, `route_tree.go`) |
| Operations, service registry, lifecycle, HTTP handler | `kernel/` (`kernel.go`, `builder.go`, `registry.go`, `lifecycle.go`, `handler.go`, `prepare.go`, `stream.go`) |
| Typed wrappers, population, substitution, path converters | `runtime/` (`ctx.go`, `request.go`, `response.go`, `stream.go`, `populate.go`, `substitute.go`, `pathconvert.go`) |
| Stream adapters + transport registry | `streaming/` (`stream.go`, `registry.go`) |
| New wire transport (SSE, NDJSON, WebSocket) | `transport/<name>/` for the server side, `client/` for the client side (`streams.go`, `websocket.go`) |
| Interceptor primitives + chains | `middleware/` (`interceptor.go`) |
| Stream event envelope | `event/` (`event.go`) |
| AuthN / AuthZ | `auth/` (`JWTValidator`, `StaticAuthorizer`, `FileWatcherAuthorizer`); compose as `middleware.UnaryInterceptor` values |
| Error codes / error type | `codes/` (enum) and `status/` (error type) |
| Generator emission | `protoc-gen-zararpc/` (`generate.go` orchestration, `handler.go` handler emission, `endpoint.go` endpoint emission, `path.go` path helpers, `gateway.go` gateway emission, `grpc.go` grpc adapter emission) |
| HTTP header constants | `metadata/` (never string literals in runtime/client/kernel/routing/encoding/examples) |

Tests follow the source file (see the `unit-test-file-convention` spec): every `foo.go` has a `foo_test.go` holding its unit tests AND benchmarks (`routing/mux_test.go`, `kernel/handler_test.go`, `client/client_test.go`, ...). Cross-cutting files (`testmain_test.go` for goleak, `parity_test.go`, `interceptor_integration_test.go`) are named by purpose. Every test package has a `testmain_test.go` calling `goleak.VerifyTestMain(m)`; a goroutine leak anywhere fails the suite. After generator changes: rebuild the plugin, `buf generate`, rebuild both modules, and run the example client end-to-end.

## Key decisions

- **grpc-free core, except `grpcbridge/`.** The framework root module (`runtime/`, `client/`, `codes/`, `encoding/`, `metadata/`, `peer/`, `status/`, `kernel/`, `routing/`, `middleware/`, `streaming/`, `transport/`, `event/`, `protoc-gen-zararpc/`) never imports `google.golang.org/grpc` — the only exception is `grpcbridge/`, which exists so the generated `.grpc.go` files stay thin. gRPC lives in the `examples/` module (generated gateway + adapter code + demo). Error-code conversion between zararpc and grpc is a direct cast because the numeric values are identical (0-16).
- **Error model mirrors grpc-go.** `codes/` holds the enum (`codes.Code`, `codes.CodeInternal`); `status/` holds the error type (`status.NewErrorf`, `status.Code`, `status.FromError`, `status.FromHTTP`, `status.FromGRPCStatus`). `FromGRPCStatus` uses reflection (`reflect.Value.Uint()`) to handle grpc's `uint32`-based `codes.Code`; the root-module test uses a `uint32` fake to guard against the signed-type trap.
- **Naming decisions (landed).** `Endpoint` -> `Operation` (`kernel.Operation`, `Register_GetUser` returns `*kernel.Operation`); `Interceptor` -> `UnaryInterceptor` / `StreamInterceptor` (`middleware.ChainUnaryInterceptors`, `middleware.ChainStreamInterceptors`, `routing.WithMuxUnaryInterceptors`, `routing.WithMuxStreamInterceptors`). RPC name constants use the `_Method` suffix; the zararpc client is `UsersServiceHTTPClient` (avoids collision with grpc-go's `UsersServiceClient`). The full canonical vocabulary is in `docs/naming-conventions.md`.
- **Health and reflection are not framework packages.** The demo server uses grpc-go's standard `reflection.Register(grpcServer)` for grpcurl/buf discovery; there is no `health/` or `reflection/` package in the framework root (they were deleted as over-engineered).
- **PGO measured, not adopted.** A `default.pgo` generated from the mux + proto benchmarks showed no reproducible win with `-pgo=auto` (allocs identical, ns delta within noise). Do not re-measure or commit a profile unless the hot path changes significantly; document `-pgo=auto` for consumers with larger workloads.
- **No arena in the hot path.** A slab arena for handler-visible data was studied and rejected: `Arena.Free()` corrupts messages retained by handlers, and every framework allocation escapes by design (params map, query `url.Values`, body bytes, `Request[T]`, message, `Response[T]` are all handed to user code that may retain them). Arena primitives are only safe when the allocator owns the lifetime. Do not pool handler-visible bytes on the mux hot path (the `sync.Pool` boxing tax costs +1 alloc/op; use `internal/xsync.Pool[T]` and `internal/xsync.Bytes` where pooling is safe).
- **Allocation budget (hot path).** Measured on darwin/arm64, Go 1.27: `BenchmarkMuxGet` ~20 allocs / ~2.4 KB, `BenchmarkMuxPost` ~18 allocs / ~2.2 KB, `BenchmarkMuxExactManyRoutes` ~15 allocs / ~1.6 KB, `BenchmarkClientDoUnary` ~78 allocs / ~7.0 KB. A regression of more than 1 alloc/op on these is a bug, not noise. The machine-checkable contract lives in the `allocation-budget` spec.