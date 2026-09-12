# AGENTS.md

Guidance for AI agents and humans working in this repository.

## Project

`zara-rpc` is a Go RPC framework: connect-go-style typed handlers over pure `net/http`, with grpc-gateway-style declarative HTTP endpoints from `google.api.http` annotations. It is **two Go modules** (Go 1.27, darwin/arm64): the framework root (grpc-free) and `examples/` (gRPC + gateway demo) which depends on the root via a `replace` directive. The code generator lives in the root module. `examples/` is a **git submodule** pointing at `https://github.com/aldok10/zara-rpc-examples` — the example code lives in its own repository.

## Commands

```sh
# Fresh clone: initialize the examples submodule
git submodule update --init --recursive

go build ./...            # build framework + generator (root module, must exit 0)
go test -count=1 ./...    # run tests (root module only)
go vet ./...              # static checks

# Build/test the examples module (separate go.mod, submodule checkout)
cd examples && go build ./... && go vet ./...

# Regenerate example code (after changing the generator or the proto)
go build -o "$(go env GOPATH)/bin/protoc-gen-zararpc" ./protoc-gen-zararpc
cd examples && buf generate && cd ..

# Run the example stack (from examples/)
cd examples
go run ./server    # combined gRPC + HTTP on :8080
go run ./gateway   # REST -> gRPC gateway on :8081
go run ./client    # end-to-end demo
```

## Repository layout

The layout mirrors `grpc-ecosystem/grpc-gateway`: the framework core lives
in responsibility-first packages (`kernel/`, `routing/`, `runtime/`,
`streaming/`, `transport/`, `middleware/`, `event/`), the code generator is
a `package main` at the repo root (`protoc-gen-zararpc/`), and public
subpackages hold cross-cutting concerns, with functional options
(`WithXxx`) for configuration. There are NO Go files at the module root.

```
protoc-gen-zararpc/        code generator (protoc plugin), package main at repo root.
                          A superset of protoc-gen-go: emits .pb.go (via protobuf-go's
                          internal generator), patches custom struct tags into it, then
                          emits .zararpc.go (handlers/clients/routes), .gateway.go
                          (REST->gRPC), and .grpc.go (gRPC server adapter). No separate
                          protoc-gen-go/zararpc-tags step.
  main.go                 plugin entry, http rule parsing, .pb.go emission + tag patch
  generate.go             orchestration: generateFile, collectTags/customTag/oneofTag,
                          generateService, generateClient
  handler.go              handler + request-builder emission (generateHandlerMethod, ...)
  endpoint.go             operation constructor emission (generateOperationFn + variants)
  path.go                 http rule parsing + path-param extraction helpers
  gateway.go              .gateway.go emission (generateGatewayFile/Service/Method)
  grpc.go                 .grpc.go emission (generateGRPCFile/Service/MethodHandler)
  patch.go                AST patcher: merges custom tags into generated .pb.go structs
  options.go              hand-written zara.options.tags/oneof_tags extensions (50001/50002)
auth/                     authentication + authorization interceptors (grpc-go authz style):
                          JWTValidator (HS256/RS256/ES256), Claims, StaticAuthorizer +
                          FileWatcherAuthorizer (Envoy-style RBAC policy, hot reload)
kernel/                   operation + service + lifecycle: Operation, OperationBuilder,
                          Service, NewService, Server (lifecycle), Handler (HTTP adapter),
                          PrepareOperation, typed stream adapters. The handler touches
                          ~14 unexported Operation fields, so it lives here, not in routing/
routing/                  route matching + dispatch: Mux, NewMux, WithMux* options,
                          Pattern, routeTree. Exact routes hash-indexed, wildcard scanned
runtime/                  typed wrappers + helpers: Ctx, Request[T], Response[T], Spec,
                          stream types, PopulateMessage, SubstitutePathParams, path
                          converters. Leaf of the core DAG (imports codes/encoding/
                          metadata/peer/status only)
streaming/                stream adapters (transport <-> type-erased Stream) + registry
transport/                wire-protocol adapters: sse/, ndjson/, websocket/ (no grpc/ or
                          rest/ placeholders)
middleware/               interceptor primitives: UnaryFunc, UnaryInterceptor, Stream,
                          StreamInterceptor, ChainUnaryInterceptors, ChainStreamInterceptors
event/                    stream event envelope (Name, ID, Retry, Data)
grpcbridge/               grpc-go bridge for generated .grpc.go (CtxFor, ToGRPC, stream
                          adapters). THE ONLY grpc-importing package in the root module —
                          see the grpc-free convention note below
client/                   HTTP client core (DoUnary, DoServerStream, DoClientStream, DoBidiStream):
                          client.go (config/options/ClientBase), unary.go, streams.go, websocket.go
codes/                    RPC error code enum (gRPC-compatible), leaf dependency
encoding/                 JSON, JSONV2, XML, Protobuf codecs + registry
internal/xsync/           strongly-typed sync wrappers: Pool[T], Map[K,V], Set[K],
                          AtomicFloat64 + Buffer/Bytes byte-buffer pools (buffers.go)
internal/xunsafe/         unsafe toolkit : Addr, VLA, SliceToString/StringToSlice (zerocopy), IsDirect, layout
metadata/                 RequestMeta: headers, query, body, cookies in context; HTTP header
                          name/value constants; HeaderPairs (HTTP headers -> gRPC metadata)
peer/                     client peer info
status/                   RPC errors built on codes (NewErrorf, Code, FromHTTP, FromGRPCStatus)
docs/                     navigation docs: architecture.md (entry points + dependency flow),
                          codebase-map.md (folder-by-folder map), naming-conventions.md
                          (canonical vocabulary + naming rules + scriptable checks)
examples/                  git submodule -> github.com/aldok10/zara-rpc-examples:
                           separate Go module (github.com/aldok10/zara-rpc-examples,
                           replace => ../) holding the gRPC + gateway demo
  go.mod                  module + replace directive
  proto/users/v1/         users.proto + generated .pb.go/.zararpc.go/.gateway.go/.grpc.go
  server/                 combined gRPC+HTTP server, JWT+RBAC auth demo, gRPC via the
                          generated adapter + grpc-go reflection.Register
  gateway/                REST-to-gRPC gateway
  client/                 end-to-end client demo (all transports, mints JWTs)
third_party/              vendored google/api annotations for offline protoc
```

The core DAG: `runtime/` is the leaf (imports only `codes/`, `encoding/`,
`metadata/`, `peer/`, `status/`); `kernel/` imports `runtime/`; `routing/`
imports `kernel/` + `runtime/` + `streaming/` + `transport/`; `streaming/`
imports `runtime/`; `transport/` imports `runtime/` + `streaming/`;
`middleware/` imports `runtime/` + `kernel/`. No package imports a package
that (transitively) imports it back. `client/` imports `runtime/` +
`middleware/` only — never `routing/` or `kernel/` (that would be a cycle).

`kernel/` holds the operation/service/lifecycle core: `kernel.go`
(`Operation` type, options, constructors), `builder.go`
(`OperationBuilder`), `registry.go` (`Service`/`ServiceBuilder`/`NewService`),
`lifecycle.go` (`Server`), `handler.go` (HTTP adapter), `prepare.go`
(`PrepareOperation`), `stream.go` (typed stream adapters), `runtime.go`
(elemType + accessors). `routing/mux.go` holds `Mux`; `runtime/` holds the
typed wrappers (`ctx.go`, `request.go`, `response.go`, `stream.go`),
`populate.go`, `substitute.go`, `pathconvert.go`. `doc.go` documents each
package; `version.go` holds `Version`.

The subpackages are public (no `internal/` restriction) and are imported directly — there are NO root re-exports and no root package. Users import `runtime`, `codes`, `status`, `encoding`, `metadata`, `peer`, `client`, `kernel`, `routing`, `middleware` where they need them.

**Test layout follows the source file (see the `unit-test-file-convention` spec):** every `foo.go` has a `foo_test.go` holding its unit tests AND benchmarks; cross-cutting files (`testmain_test.go` for goleak, `parity_test.go`, `interceptor_integration_test.go`) are named by purpose and never absorb a source file's unit tests. Examples: `routing/mux_test.go` (mux unit tests + `BenchmarkMux*`), `kernel/handler_test.go`, `runtime/populate_test.go`, `client/client_test.go`, `client/sse_leak_test.go`. Every test package with tests has a `testmain_test.go` calling `goleak.VerifyTestMain(m)` — a goroutine leak anywhere fails the suite. The client tests use plain `httptest` servers — the client package cannot import the routing Mux (that would be an import cycle).

## Codegen workflow

1. Edit `examples/proto/users/v1/users.proto` (or the generator).
2. Rebuild the plugin: `go build -o "$(go env GOPATH)/bin/protoc-gen-zararpc" ./protoc-gen-zararpc`.
3. Regenerate: `cd examples && buf generate`. The generator emits the `.pb.go` files itself (with custom struct tags patched in) — there is no separate protoc-gen-go or zararpc-tags step.
4. `go build ./...` (root) AND `cd examples && go build ./...` must pass — the generated code is part of the examples module.

Generated files are committed (the example must build without running protoc). Never hand-edit `*.zararpc.go`, `*.gateway.go`, `*.pb.go`, `*_grpc.pb.go` — the only exception is the generator rewriting struct tags in `*.pb.go` (generator-driven, not hand-editing).

## Conventions & decisions

- **Core is grpc-free, except `grpcbridge/`.** The framework root module (`runtime/`, `client/`, `codes/`, `encoding/`, `metadata/`, `peer/`, `status/`, `kernel/`, `routing/`, `middleware/`, `streaming/`, `transport/`, `event/`, `protoc-gen-zararpc/`) never imports `google.golang.org/grpc` — the only exception is `grpcbridge/`, which exists so the generated `.grpc.go` files stay thin (they import `grpcbridge.CtxFor`/`ToGRPC` instead of inlining the bridge). gRPC lives in the `examples/` module (generated gateway + adapter code + demo). Error-code conversion between zararpc and grpc is a direct cast (`codes.Code(grpcstatus.Code(err))`) because the numeric values are identical (0–16).
- **Error model mirrors grpc-go.** `codes/` holds the enum (`codes.Code`, `codes.CodeInternal`); `status/` holds the error type (`status.NewErrorf`, `status.Code(err)`, `status.FromError`, `status.FromHTTP`, `status.FromGRPCStatus`). There is no root alias — import `codes` and `status` directly. `FromGRPCStatus` uses reflection (`reflect.Value.Uint()`) to handle grpc's `codes.Code` which is `uint32`-based — the root-module test uses a `uint32` fake to guard against the signed-type trap.
- **Naming (user decisions, final):** operation constructors `Register_GetUser` return `*kernel.Operation`, registration `RegisterUsersServiceRoutes`, RPC name constants `_Method` suffix, zararpc client `UsersServiceHTTPClient` (avoids collision with grpc-go's `UsersServiceClient`). `Endpoint` was renamed to `Operation`; `Interceptor` to `UnaryInterceptor`/`StreamInterceptor` (chains: `ChainUnaryInterceptors`/`ChainStreamInterceptors`; mux options: `WithMuxUnaryInterceptors`/`WithMuxStreamInterceptors`). Legacy names `WithOperationCodec`/`WithOperationUnaryInterceptors` remain as deferred renames. HTTP wrapper `Handler` in kernel/handler.go stays. The full canonical vocabulary is in `docs/naming-conventions.md`.
- **OperationBuilder: zero-allocation operation construction.** Generated `.zararpc.go` builds operations with `var op kernel.Operation; (*kernel.OperationBuilder[Req, Res])(&op).SetMethod(...).SetUnaryHandler(...).Build()` — the builder is the underlying type of `Operation` (identical underlying types, so the conversion is free), setters mutate the struct in place and return the builder for chaining, and `Build()` is the single point where the struct escapes to the heap. The `NewOperation`/`NewServerStreamOperation`/`NewClientStreamOperation`/`NewBidiStreamOperation` constructors (functional options) remain for hand-written code but allocate before options apply; the builder is the canonical construction path for generated code. `kernel/builder.go` holds the builder; `kernel/kernel.go` holds the `Operation` type, options, and constructors.
- **Gateway helpers live in the framework, not generated code.** The generated `.gateway.go` calls `metadata.HeaderPairsFromMeta(ctx.Meta())` (HTTP headers → gRPC metadata payload, hop-by-hop filtered, keys lowercased) and `status.FromGRPCStatus(err)` (grpc error → zararpc error via reflection, grpc-free). The old per-package `forwardHeaders`/`toZaraError` emission was removed — do not reintroduce it; a package with multiple proto files would get duplicate symbols. `HeaderPairs(ctx)` remains for plain contexts; the gateway uses the `FromMeta` variant because `runtime.Ctx` keeps metadata in its state, not in the wrapped context.
- **HTTP header constants live in `metadata`.** The stdlib exports no header-name constants, so `metadata` defines `HeaderContentType`, `HeaderAuthorization`, `HeaderAccept`, etc. plus common values (`ContentTypeJSON`, `ContentTypeEventStream`, `ContentTypeGRPC`, ...). Use them instead of string literals in runtime/client/kernel/routing/encoding/examples.
- **`internal/xunsafe`** `Addr[T]` stores `*T` (not an intptr) so `AssertValid` is a plain field read — the upstream intptr form fails `go vet`'s unsafeptr check. Consequences: `Addr` cannot be converted between element types (convert via `unsafe.Pointer`), `NoEscape` is an identity (the uintptr-XOR trick fails vet; do not rely on it for allocation control), and `SignBitMask` was dropped (constructing an invalid pointer is not vet-clean). `SliceToString`/`StringToSlice` ARE the zerocopy helpers — there is no separate `internal/zerocopy`. The toolkit has no hot-path call sites: the compiler already optimizes safe conversions and framework allocations escape by design; it is used in tests (`IsDirect` locks the boxing-tax lesson).
- **Streaming transports:** server streaming → SSE (default) or WebSocket (`WithServerStreamTransport`), client streaming → NDJSON, bidi → WebSocket (`github.com/coder/websocket`).
- **Mux routing split:** exact routes (no `{` in the path) are hash-indexed in `Mux.exact` (method → path → handler) for O(1) routing; wildcard routes live in `Mux.wildcard` sorted by specificity and are scanned per request. First registration wins for duplicate exact paths. When adding a route, classify by `strings.Contains(path, "{")` — do not put exact routes in the wildcard slice.
- **WS codec negotiation:** WebSocket dials carry no Content-Type, so clients append `?encoding=<codec-name>`; server honors it via `codecFromRequest`. `CodecForRequest(ct, accept)` handles Accept for bodyless HTTP requests.
- **Binary safety:** `Codec` has `IsBinary() bool`. SSE/NDJSON base64-encode binary codecs (protobuf); WS uses binary frames for binary codecs.
- **Handlers receive `runtime.Ctx`, not `context.Context`.** Generated handler signatures are `GetUser(ctx runtime.Ctx, req *GetUserRequest) (*User, error)`. `Ctx` carries the transport metadata (headers, query, body, cookies, path params), the negotiated codec, the protocol (`"http"`/`"grpc"`/`"websocket"`), the operation spec, the client peer, and the typed payloads (`ctx.Request[T]()`, `ctx.Response[T]()`). It satisfies `context.Context` (deadline/cancellation/values delegate to the wrapped context); `ctx.Context()` returns the plain context for APIs that need it (e.g. the gateway's grpc client calls). `Ctx` is passed by value through the interceptor chain; the shared `*ctxState` makes `WithValue`/`SetRequest`/`SetResponse` visible to all copies. The metadata lives in the state, NOT in the wrapped context — `metadata.*FromContext` helpers return empty on a `Ctx`; use `ctx.Header()`/`ctx.Query()`/`ctx.Cookie()`/`ctx.Body()` instead. `NewCtx` deliberately skips `metadata.WithRequestMeta` so the hot path pays one allocation (the state), not two.
- **Auth package (grpc-go authz style).** `auth/` holds the default authentication + authorization layer: `auth.NewJWTValidator(secret)` (HS256) or `NewJWTValidatorWithKey(key)` (RS256/ES256) validates JWTs and attaches `auth.Claims` to the Ctx; `auth.NewStatic(policyJSON)` / `auth.NewFileWatcher(policyFile, refresh)` enforce an Envoy-style RBAC policy (allow/deny rules over role claims, sub, source IP, headers, and the RPC path). Compose them as `middleware.UnaryInterceptor` values via the `auth/interceptor` package: `interceptor.Public(isPublic)` marks a request public (JWT/RBAC skip it), `interceptor.JWT(v)` validates, `interceptor.RBAC(a)` authorizes — chain them as `routing.WithMuxUnaryInterceptors(interceptor.Public(isPublic), interceptor.JWT(v), interceptor.RBAC(a))`. The package is grpc-free and reads transport payloads from `runtime.Ctx`, so one chain protects HTTP, WebSocket, and gRPC uniformly. The example server runs the same chain on the single HTTP mux AND via grpc interceptors (`authUnaryInterceptor`/`authStreamInterceptor` in examples/server) — the gRPC adapters set the procedure spec per method (`grpcCtxFor`) so RBAC path matching works over gRPC. `middleware.ChainUnaryInterceptors` is exported for composing chains outside the mux.
- **Streaming endpoints run the interceptor chain too.** `Handler.streamAuth` runs the chain (without a handler) before an SSE/NDJSON/WebSocket stream is established; a rejected request gets the error response before any stream bytes. This was added so authz protects streaming transports, not just unary.
- **Client WS dials forward headers.** `websocket.Dial` receives `HTTPHeader: cfg.headers` in `doServerStreamWS` and `DialWebSocket` — without it, per-call `client.WithHeader(Authorization, ...)` never reached the WS handshake and the server rejected the stream with 401.
- **NDJSON is newline-delimited on the client too.** `ndjsonWriteStream.Send` appends `'\n'` for text codecs (binary codecs base64-encode per line). The server reads with `ReadBytes('\n')`; without the newline, messages concatenate into one line and the server sees EOF before any complete message (the demo showed `stored 0 users`).
- **Auth pattern:** handlers read transport payloads from `Ctx` (`ctx.Header()`, `ctx.Query()`, `ctx.Cookie()`, `ctx.Body()`). The example's `auth.DefaultTokenExtractor` reads Authorization header → `token` query param → `session` cookie. gRPC callers wrap the context via `grpcCtx` (`runtime.NewCtx(ctx, meta, encoding.ProtoCodec{}).WithProtocol("grpc")`) so handlers see the same API and `ctx.IsGRPC()`/`ctx.IsJsonCodec()` distinguish transports.
- **Client extra options must not leak:** `Do*` methods copy the config AND clone the headers map (`cfg.headers = c.cfg.headers.Clone()`). A per-call `client.WithHeader` must never mutate the base client.
- **Custom struct tags:** declare `(zara.options.tags)` (full tag literal, e.g. `"xml:\"flag,attr\" gorm:\"primaryKey\""`) on proto fields, and `(zara.options.oneof_tags)` on oneofs (targets the oneof field on the message struct; field-level tags on oneof members target the synthesized `Message_Field` wrapper structs). protoc-gen-zararpc patches the tags directly into the generated `.pb.go` structs (AST-based, after protoc-gen-go's engine emits the file) so reflection libraries (encoding/xml, gorm, mapstructure) see them. There is no `.tags.go` overlay and no `zararpc-tags` tool — the generator owns the `.pb.go` output. Extension names must be fully qualified (`(zara.options.tags)`) — protobuf resolves unqualified names only within the package chain. The extension definitions are hand-written in `protoc-gen-zararpc/options.go` (numbers 50001/50002, registered via `protoregistry.GlobalTypes`); the examples' vendored `proto/zara/customtag/options.proto` declares the same extensions for buf. Tag merging goes through `fatih/structtag` (the only third-party dep in the root module besides protobuf) — do not hand-roll tag parsing.
- **PGO measured, not adopted:** a `default.pgo` generated from the mux + proto benchmarks was tested with `-pgo=auto`; the hot path showed no reproducible win (allocs identical, ns delta within noise). Do not re-measure or commit a profile unless the hot path changes significantly. Consumers with larger workloads may still benefit; document `-pgo=auto` for them.

## Goroutine leak checking

Leaks accumulate silently: each leaked goroutine pins its stack and everything it references, raising GC pressure and memory use. Three layers, per the Go blog "Goroutine Leak Profiles" (go.dev/blog/goroutine-leak-profiles):

1. **Unit tests — goleak (mandatory).** Every test package has `testmain_test.go` with `goleak.VerifyTestMain(m)`. A new test that spawns goroutines (streams, timers, background pumps) must either clean them up or use `defer goleak.VerifyNone(t)` inside the test. The client's SSE body-close goroutine is the canonical case: it must exit on BOTH `ctx.Done()` and `stream.Close()` — a caller that closes without canceling the context must not leak.
2. **Production — Go 1.27 goroutine leak profiler.** Available as the `goroutineleak` profile type in `runtime/pprof`, or at `/debug/pprof/goroutineleak` when `net/http/pprof` is imported. It precisely reports goroutines permanently blocked on channels/sync primitives (GC-reachability based, near-zero false positives). Collect with `curl localhost:6060/debug/pprof/goroutineleak > leak.prof` and inspect with `go tool pprof leak.prof`; `list <fn>` shows the blocking site. Run it periodically on long-lived servers — leaks that are rare per request still accumulate.
3. **Patterns that leak (from the blog, all seen in real code):** unbuffered channel send after early return (fix: buffer the channel), `select` timeout that strands a sender (fix: buffer 1), `range` over a never-closed channel (fix: `close`), missing `Unlock` before `break`, `WaitGroup.Wait` inside the spawn loop, send-on-channel instead of `close` while holding a lock. When reviewing streaming code, ask: what unblocks this goroutine, and can that condition always happen?

## Memory & CPU budget (hot path)

The mux hot path is benchmarked in `routing/mux_test.go`; the client unary path in `client/client_test.go`. Current budget (darwin/arm64, Go 1.27): `BenchmarkMuxGet` ~20 allocs / ~2.4 KB, `BenchmarkMuxPost` ~18 allocs / ~2.2 KB, `BenchmarkMuxExactManyRoutes` ~15 allocs / ~1.6 KB, `BenchmarkClientDoUnary` ~78 allocs / ~7.0 KB. Treat a regression of more than 1 alloc/op on these as a bug. The machine-checkable copy of this contract is the `allocation-budget` spec (`openspec/specs/allocation-budget/spec.md` after archive; the delta lives in `openspec/changes/perf-memory-allocation/`). The `runtime.Ctx` change improved the mux by 1 alloc/op: the old `metadata.WithRequestMeta` cost two allocations (context wrapper + interface boxing of `RequestMeta`), while the shared `*ctxState` costs one. `BenchmarkMuxGetGenerated` (same URL as `BenchmarkMuxGet`, but a request builder that assigns fields directly like the generated `.zararpc.go`) shows the reflection elimination is a CPU win, not an alloc win: ~20 allocs / ~1.1 µs vs ~1.3 µs — the alloc count is at the contract floor because every handler-visible value escapes by design.

- **Every hot-path allocation escapes to the handler by design.** The params map, query `url.Values`, body bytes, `Request[T]`, message, and `Response[T]` are all handed to user code that may retain them. This is the framework's contract — do not pool or arena-free them.
- **Operation construction is zero-allocation via the builder.** Generated `.zararpc.go` builds operations with `var op runtime.Operation; (*runtime.OperationBuilder[Req, Res])(&op).SetMethod(...).SetUnaryHandler(...).Build()` — the conversion is free (identical underlying types), setters mutate the struct in place, and `Build()` is the single point where the struct escapes to the heap. The `NewOperation` family (functional options) allocates before options apply and is for hand-written code only. Do not reintroduce constructor churn in generated code.
- **No arena in the hot path (decided).** A slab arena for handler-visible data was studied and rejected: `Arena.Free()` corrupts messages retained by handlers, and every framework allocation escapes. Arena primitives (`VLA`, `Addr`, `NoEscape`, `StoreNoWB`) are only safe when the allocator owns the lifetime — true for a codec, false for a framework. Do not reintroduce an arena without changing the retention contract.
- **What IS safe:** (1) avoid boxing — `setField` takes typed `string`/`[]string`, never `any` (interface boxing allocates for non-pointer-shaped values); (2) parse the query once and pass it down (`codecFromRequest(r, query)`); (3) attach metadata to the context once (`metadata.WithRequestMeta`) and pass `ctx` down — never `r.WithContext` (copies the whole request); (4) keep only live context values — `requestInfo` was removed because `PathParamsFromContext`/`SpecFromContext` had zero callers; (5) string fast paths in `SubstitutePathParams`/`MessageToQuery`; (6) `Mux.exact` hash index for exact routes; (7) cache the operation `Spec` once — `Operation.specCache` is computed after the options are applied and refreshed by `Mux.Register` (which sets the service-qualified procedure and prefixed path); the hot path must never reconstruct the Spec (or its `"/"+RPC` concatenation when Procedure is empty) per request.
- **sync.Pool boxing tax (measured):** a raw `sync.Pool` of `[]byte` allocates the slice header on every `Put` — `sync.Pool.Put` takes `any`, and boxing a non-pointer-shaped value allocates. This showed up as +1 alloc/op on the mux benchmark when response bytes were pooled. The fix is `internal/xsync.Pool[T]`, which stores `*T`: a pointer boxes for free. `internal/xsync.Bytes` hands out `*[]byte` for this reason — do not revert it to a raw `[]byte` pool. `*bytes.Buffer` pools were always free (pointer-shaped).
- **Do not pool handler-visible bytes on the mux hot path.** The encoded response body is the largest per-request allocation, but returning it to a pool costs the boxing tax above, and `Marshal` still allocates — the benchmark regressed 21→22 with no alloc-count win. The streaming base64 buffers ARE pooled because the reused buffer is large enough that the GC-pressure win outweighs the header tax.
- **Do not stream-encode unary responses via `codec.NewEncoder`.** `JSONCodec.Marshal` switches to protojson for `proto.Message`; `NewEncoder` returns a plain `json.Encoder` that would render protobuf messages with Go field names. The intermediate `[]byte` from `Marshal` is the price of correct proto encoding.
- **GC pressure is minimized by allocation count, not GC tuning.** Do not set `GOGC`/`GOMEMLIMIT` in framework code; consumers tune their own runtime. Keep per-request allocations small and few, and keep goroutine lifetimes bounded (see leak checking above).

## Verification loop

- After any edit: run `go build ./...` and `go test -count=1 ./...` yourself and cite the output. Do not claim "it works" without running it. For examples-module edits, also run `cd examples && go build ./...`.
- After generator changes: rebuild the plugin, regenerate, rebuild both modules, and run the example client end-to-end (it exercises JSON, Protobuf, SSE, NDJSON, and WebSocket).
- The example server requires auth on GetUser: a valid JWT signed with the generated secret / `JWT_SECRET` (role `admin` or `user`) returns 200, no token returns 401, and a reader token on DeleteUser returns 403. Verify via curl, grpcurl, and the gateway. The client demo mints tokens with `mintToken(role)`.
- After hot-path changes: run the mux + client benchmarks and compare allocs/op against the budget above. A regression of more than 1 alloc/op is a bug, not noise.
- After streaming changes: the goleak suite must pass (every test package has `goleak.VerifyTestMain`). For long-lived server changes, collect a `goroutineleak` profile from a running instance and confirm zero leaked goroutines.

## Known gotchas

- **Stale LSP errors:** gopls often reports `undefined: RegisterUsersServiceRoutes`, `NewUsersServiceHTTPClient`, `NewOperation`, or `missing method IsBinary` on files that actually compile. Trust `go build ./...`, not the LSP.
- **WS server-streaming hang:** `WatchUsers` only streams EXISTING users. A fresh server with an empty store produces zero events and the client blocks. This is expected behavior, not a bug — create users first.
- **Benchmark methodology:** `httptest.NewRequest` serializes and re-parses the request (bufio reader, header clone, readRequestLimit) — build it ONCE outside the loop or it dominates the measurement. A reused request body is consumed by the first iteration; reset `req.Body` per iteration or the benchmark silently measures the ERROR path (writeError dominates the alloc profile). The mux benchmarks in `routing/mux_test.go` follow both rules.
- **grpcurl client-streaming quirk:** multiple `-d` flags only send one message; the server is correct (the Go gRPC client reports the true count).
- **Two modules, no workspace:** the root module and `examples/` are separate Go modules linked by a `replace github.com/aldok10/zara-rpc => ../` directive in `examples/go.mod` (mirroring grpc-gateway). Do not create a `go.work` file, and do not add a `go.mod` inside `protoc-gen-zararpc/`.
- **Examples is a submodule:** `examples/` is a gitlink to `github.com/aldok10/zara-rpc-examples`. Changes to example code happen in that repo and are pinned here via the gitlink commit. The `replace => ../` only resolves when the submodule is checked out inside the parent repo (the normal workflow); a standalone clone of the examples repo needs the replace path adjusted.
- **Patched .pb.go is a build artifact:** `protoc-gen-zararpc` rewrites struct tags in `*.pb.go` (it owns the .pb.go output — `buf.gen.yaml` has no protoc-gen-go). The examples' `tags_test.go` (`TestStructNamedCustomTags`) catches a dropped patch — run the examples tests after any regeneration.
- **Permission rules:** `rm -rf` on absolute paths may be blocked by the sandbox; use targeted deletes or leave temp dirs under `/tmp` alone.

## Task flow (OpenSpec is the source of truth)

**OpenSpec is the project's task-flow management and source of truth.** Every change to this repository SHALL be tracked as an OpenSpec change under `openspec/changes/<name>/` with `proposal.md`, `design.md`, `specs/<capability>/spec.md`, and `tasks.md`. The `openspec/` tree (active changes + archived specs) is the authoritative record of what the project is, what it decided, and what is in flight — not this file, not the git log, not the todo list.

- Before starting multi-step work: create or update the OpenSpec change (`openspec change new` / edit the artifacts), keep one task in progress, and mark tasks `[x]` only after verification passes.
- When implementation diverges from the spec (a decision, a removed package, a renamed symbol), update the spec in the same change — contract consistency over code.
- When a change touches the generator, the proto, and the example, treat regeneration + full example run as part of "done".
- Archive completed changes with `openspec archive <name>`; archived specs under `openspec/specs/` are the durable project contract.
- The `unit-test-file-convention`, `core-layout`, `allocation-budget`, and `naming-conventions` specs are the machine-checkable contracts for test layout, package layout, hot-path budget, and vocabulary — run their checks after relevant edits.