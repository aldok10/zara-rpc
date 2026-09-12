# Codebase map

One line per top-level directory (plus key subdirectories). Responsibilities are discoverable by folder and file name; see `docs/architecture.md` for dependency flow and `docs/naming-conventions.md` for naming rules.

| Directory | Responsibility |
|---|---|
| `auth/` | Authentication + authorization interceptors: `JWTValidator` (HS256/RS256/ES256), `Claims`, `StaticAuthorizer` + `FileWatcherAuthorizer` (Envoy-style RBAC policy, hot reload); grpc-free, reads transport payloads from `runtime.Ctx`. Files: `jwt.go`, `claims.go`, `rbac.go`, `policy.go`. |
| `client/` | HTTP client core for generated clients: `ClientBase`, `DoUnary`, `DoServerStream`, `DoClientStream`, `DoBidiStream`, `DialWebSocket`. Files: `client.go` (config + options), `unary.go`, `streams.go`, `websocket.go`. |
| `codes/` | RPC error code enum (`codes.Code`, gRPC-compatible numeric values 0-16); leaf dependency. |
| `encoding/` | Codec interface + implementations (JSON, JSONV2, XML, Protobuf) + registry; `Codec.IsBinary()` drives SSE/NDJSON base64 and WS binary frames. Files: `codec.go`, `json.go`, `jsonv2.go`, `xml.go`, `proto.go`. |
| `event/` | Stream event envelope: `Event` (Name, ID, Retry, Data); `transport/sse` renders envelopes when the message is `*event.Event`. |
| `grpcbridge/` | grpc-go bridge for generated `.grpc.go`: `CtxFor` (context -> `runtime.Ctx` with procedure spec), `ToGRPC` (zararpc error -> grpc status), stream adapters. THE ONLY grpc-importing package in the root module. |
| `internal/xsync/` | Strongly-typed sync wrappers (internal only): `Pool[T]`, `Map[K,V]`, `Set[K]`, `AtomicFloat64`, `Buffer`/`Bytes` byte-buffer pools. Files: `pool.go`, `map.go`, `set.go`, `atomic.go`, `buffers.go`. |
| `internal/xunsafe/` | Unsafe toolkit (internal only): `AddrOf`, `VLA`, `SliceToString`/`StringToSlice` (zerocopy), `IsDirect`, layout. Files: `addr.go`, `vla.go`, `slice.go`, `any.go`, `escape.go`, `pointer.go`, `untyped.go`, `pc.go`, `xunsafe.go`. |
| `kernel/` | Operation + service + lifecycle core: `Operation` (type, options, constructors), `OperationBuilder` (zero-alloc construction), `Service`/`ServiceBuilder`/`NewService`, `Server` (lifecycle), `Handler` (HTTP adapter), `PrepareOperation`, typed stream adapters. Files: `kernel.go`, `options.go`, `builder.go`, `registry.go`, `lifecycle.go`, `handler.go`, `prepare.go`, `stream.go`, `runtime.go`, `doc.go`. |
| `metadata/` | `RequestMeta` (headers, query, body, cookies, params) in context; HTTP header name/value constants (`HeaderContentType`, ...); `HeaderPairs` / `HeaderPairsFromMeta` (HTTP headers -> gRPC metadata). |
| `middleware/` | Interceptor primitives: `UnaryFunc`, `UnaryInterceptor`, `Stream`, `StreamInterceptor`, `ChainUnaryInterceptors`, `ChainStreamInterceptors`. Files: `interceptor.go`, `doc.go`. |
| `peer/` | Client peer info (`peer.Peer`: address, TLS state); imported by `runtime`. |
| `protoc-gen-zararpc/` | Code generator (protoc plugin, package main): emits `.pb.go` (via protobuf-go's internal generator) with custom struct tags patched in, then `.zararpc.go`, `.gateway.go`, and `.grpc.go`. Files: `main.go` (plugin entry, http rule parsing), `generate.go` (orchestration), `handler.go` (handler + request builder emission), `endpoint.go` (endpoint function emission), `path.go` (path param extraction helpers), `gateway.go` (`.gateway.go` emission), `grpc.go` (`.grpc.go` emission), `patch.go` (AST tag patcher), `options.go` (custom tag extensions 50001/50002). |
| `routing/` | Route matching + dispatch: `Mux` (exact routes hash-indexed, wildcard scanned), `NewMux`, `WithMux*` options, `Pattern` (path templates), `routeTree`. Files: `mux.go`, `pattern.go`, `route_tree.go`, `doc.go`. |
| `runtime/` | Typed wrappers + helpers (leaf of the core DAG): `Ctx`, `Request[T]`, `Response[T]`, `Spec`, stream types, `PopulateMessage`, `SubstitutePathParams`, path converters. Files: `ctx.go`, `request.go`, `response.go`, `stream.go`, `populate.go`, `substitute.go`, `pathconvert.go`, `doc.go`, `version.go`. |
| `status/` | RPC error type built on `codes`: `NewErrorf`, `Code`, `FromError`, `FromHTTP`, `FromGRPCStatus`. |
| `streaming/` | Stream adapters (transport <-> type-erased `Stream`) + transport registry. Files: `stream.go`, `registry.go`, `doc.go`. |
| `transport/` | Wire-protocol adapters: `sse/` (`SSEServerStream`), `ndjson/` (`NDJSONClientStream`), `websocket/` (`AcceptWebSocket`, `WSConn`, `WSServerStream`, `WSBidiStream`). No `grpc/` or `rest/` placeholders. |
| `examples/` | Git submodule -> github.com/aldok10/zara-rpc-examples; separate Go module (`replace => ../`) holding the gRPC + gateway demo. Subdirs: `server/` (combined gRPC+HTTP server, JWT+RBAC auth demo, gRPC via generated adapter + grpc-go `reflection.Register`), `gateway/` (REST-to-gRPC gateway), `client/` (end-to-end client demo), `proto/users/v1/` (users.proto + generated `.pb.go`/`.zararpc.go`/`.gateway.go`/`.grpc.go`), `proto/zara/customtag/` (vendored custom-tag extension declarations for buf). |
| `third_party/` | Vendored `google/api` annotations for offline protoc. |

## File-level map of `kernel/`

| File | Responsibility |
|---|---|
| `kernel.go` | `Operation`: RPC method bound to an HTTP route (method + path + handler + body mapping); `OperationOption` + `With*` options; `NewOperation` family. |
| `builder.go` | `OperationBuilder[Req, Res]`: zero-allocation in-place operation construction (setters + `Build()`). |
| `registry.go` | `Service` / `ServiceBuilder` / `NewService`: collection of operations with name + prefix. |
| `lifecycle.go` | `Server`: HTTP server wrapper (timeouts, TLS, lifecycle). |
| `handler.go` | `Handler`: HTTP wrapper adapting `net/http` to operations; owns transport selection (SSE/NDJSON/WS). |
| `prepare.go` | `PrepareOperation`: operation preparation after options are applied. |
| `stream.go` | Typed stream adapters (`typedServerStream`, `typedClientStream`, `typedBidiStream`) used by the builder. |
| `runtime.go` | `elemType` + accessors. |

## File-level map of `routing/`

| File | Responsibility |
|---|---|
| `mux.go` | `Mux`: route registry + dispatcher; exact routes hash-indexed, wildcard routes sorted by specificity; `WithMuxUnaryInterceptors` / `WithMuxStreamInterceptors`. |
| `pattern.go` | `Pattern`: path template parsing/matching (`ParsePattern`, `Match`). |
| `route_tree.go` | `routeTree`: wildcard route storage + matching. |

## File-level map of `runtime/`

| File | Responsibility |
|---|---|
| `ctx.go` | `Ctx`, `ctxState`, `NewCtx`: handler context carrying transport metadata + codec + protocol + spec + peer. |
| `request.go` | `Request[T]`, `AnyRequest`. |
| `response.go` | `Response[T]`, `AnyResponse`. |
| `stream.go` | `StreamType`, `ServerStream[T]`, `ClientStream[T]`, `BidiStream[Req, Res]`, `ErrStreamClosed` (types only). |
| `populate.go` | `PopulateMessage`, `PopulateQuery`, `SetFieldFromBody`: request -> message population. |
| `substitute.go` | `SubstitutePathParams`, `MessageToQuery`: client-side path substitution + query conversion. |
| `pathconvert.go` | Path parameter converters: `PathString`, `PathBool`, `PathInt32`, ... `PathBytes`, `PathStringSlice`, `PathInt64Slice`. |
| `doc.go` | Package documentation. |
| `version.go` | `Version` constant. |