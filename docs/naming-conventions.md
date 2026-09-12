# Naming conventions

Canonical vocabulary, naming rules per artifact type, forbidden generic names, and scriptable checks. One concept = one vocabulary; the canonical terms below are the current code (the `Endpoint` -> `Operation` and `Interceptor` -> `UnaryInterceptor`/`StreamInterceptor` renames have landed). Legacy terms are marked and must not be used in new code.

## Canonical vocabulary

| Canonical term | Meaning | Allowed alternatives | Forbidden ambiguous names |
|---|---|---|---|
| operation | RPC method bound to an HTTP route (method + path + handler + body mapping) | endpoint (legacy, renamed) | handler, route, action |
| unary interceptor | wraps a unary RPC call | interceptor (legacy, renamed) | middleware |
| stream interceptor | wraps per-message Send/Receive on streaming RPCs | — | middleware |
| service | a collection of operations (e.g. `UsersService`) | — | module, controller |
| handler | user-implemented RPC function, or `runtime.Handler` HTTP wrapper | — | controller, action |
| mux | route registry + dispatcher | router | — |
| codec | message encoding/decoding (JSON, JSONV2, XML, Protobuf) | encoder, serializer | — |
| transport | wire protocol (SSE, NDJSON, WebSocket) | — | protocol (reserved for `ctx.Protocol()`) |
| procedure | fully-qualified RPC name (`/svc/Method`) | — | method (reserved for HTTP method) |
| spec | operation metadata (Procedure, Method, Path, Service, RPC) | — | info, descriptor |
| pattern | parsed path template | — | route, template |
| request / response | typed message wrappers (`Request[T]`, `Response[T]`) | — | dto, model, payload |
| ctx | handler context carrying transport metadata + codec + protocol + spec + peer | — | context (stdlib) |
| meta | transport metadata (headers, query, body, cookies, params) | — | — |
| peer | client peer info | — | client (reserved for the client package) |
| claims | JWT claims | — | payload |
| validator | JWT validation (`JWTValidator`) | authenticator | — |
| authorizer | RBAC policy enforcement (`StaticAuthorizer`, `FileWatcherAuthorizer`) | — | — |
| policy | RBAC allow/deny rules | — | rules |
| handler router | protocol dispatch on one listener (`HandlerRouter`, `WithGRPC`); path-level policy lives in the interceptor context (`auth/interceptor.Public`) | — | — |
| automatic query binding | fields not bound by path/body become query parameters per `google.api.http` (`runtime.PopulateQuery`, nested dot-paths included) | — | — |

## Naming rules per artifact type

| Artifact | Rule |
|---|---|
| Folders | One responsibility per folder; domain-oriented (`auth`, `client`, `codes`, `encoding`, `kernel`, `metadata`, `middleware`, `peer`, `routing`, `runtime`, `status`, `streaming`, `transport`, `event`). No generic dumping-ground folders (`utils`, `common`, ...). |
| Packages | Short, lowercase, no underscores; name states the responsibility. Internal-only packages live under `internal/`. |
| Files | One responsibility per file; the file name must reveal it (`pattern.go` holds path templates, `populate.go` holds message population). No mega-files: keep under the 500-line budget (excluding generated `*.pb.go`). |
| Types | Noun naming the concept (`Operation`, `Mux`, `Pattern`, `Codec`, `Request[T]`). Generic suffixes (`Info`, `Data`, `Manager`) are forbidden. |
| Interfaces | Name the capability, not the implementation (`Codec`, `UnaryInterceptor`, `StreamInterceptor`, `WSConn`). |
| Functions | Verb or verb phrase describing the action (`PopulateMessage`, `SubstitutePathParams`, `NewJWTValidator`). Constructors are `NewXxx`. |
| Booleans | Answer a yes/no question: `IsBinary`, `IsGRPC`, `IsJsonCodec`, `IsDirect`. No bare nouns (`binary`, `grpc`). |
| Constants | `HeaderContentType`-style: domain prefix + specific name. RPC name constants use the `_Method` suffix. |
| Errors | Built with `status.NewErrorf(code, ...)`; never bare `errors.New` for RPC failures. |
| DTOs | Use the generated protobuf message types; do not hand-write parallel `dto`/`model`/`payload` structs. |
| Tests | One test file per source file (see the `unit-test-file-convention` spec): `foo_test.go` holds the unit tests AND benchmarks for `foo.go` (`mux_test.go` tests + benchmarks `mux.go`). Cross-cutting files are named by purpose (`testmain_test.go` for goleak, `parity_test.go`, `interceptor_integration_test.go`) and never absorb a source file's unit tests. No standalone `bench*_test.go` / `*_bench_test.go` files. |

## Forbidden generic names

Never create new files, folders, or packages with these names (they are dumping grounds by construction):

- Folders: `utils`, `util`, `common`, `helpers`, `misc`, `shared`
- Files: `manager.go`, `helper.go`, `common.go`, `data.go`

## Scriptable checks

Run in verification (from the repo root; `examples/` is a separate module and is excluded):

```sh
# Package comment presence (every package has a doc.go or package comment)
for d in auth client codes encoding event grpcbridge kernel metadata middleware peer routing runtime status streaming transport internal/xsync internal/xunsafe; do
  grep -q "^// Package " "$d/doc.go" 2>/dev/null || grep -q "^// Package " "$d"/*.go 2>/dev/null || echo "MISSING: $d"
done

# Generic name scan (no dumping-ground packages/files)
find . -type d \( -name utils -o -name util -o -name common -o -name helpers -o -name misc -o -name shared \) -not -path "./.git/*" -not -path "./examples/*"
find . -name "manager.go" -o -name "helper.go" -o -name "common.go" -o -name "data.go" | grep -v examples

# File size budget (>500 lines, excluding generated .pb.go)
find . -name "*.go" -not -name "*.pb.go" -not -path "./examples/*" -not -path "./.git/*" -exec wc -l {} + | awk '$1 > 500 {print}'

# Test locality: every non-test .go file has a matching foo_test.go
# (exceptions: doc.go, version.go, testmain_test.go, and cross-cutting files
# listed explicitly below). Run from the repo root; examples/ is a separate module.
for f in $(find . -name "*.go" -not -name "*_test.go" -not -path "./examples/*" -not -path "./.git/*" | grep -v -E '/(doc|version)\.go$'); do
  base=$(basename "$f" .go)
  dir=$(dirname "$f")
  [ -f "$dir/$base"_test.go ] || echo "NO TEST: $f"
done

# No standalone benchmark files (benchmarks live in foo_test.go)
find . -name "bench*_test.go" -o -name "*_bench_test.go" | grep -v examples

# Cross-cutting test files must be named by purpose (testmain, parity,
# integration, leak) — a *_test.go without a matching source file is a
# violation unless it matches one of these patterns.
for f in $(find . -name "*_test.go" -not -path "./examples/*" -not -path "./.git/*"); do
  base=$(basename "$f" _test.go)
  dir=$(dirname "$f")
  if [ ! -f "$dir/$base.go" ] && ! echo "$f" | grep -qE '/(testmain|parity|integration|leak|bench)_test\.go$'; then
    echo "CROSS-CUTTING WITHOUT PURPOSE: $f"
  fi
done
```

A check that prints nothing (or only `MISSING` lines you are fixing) passes. The checks are grep-able by design so drift is cheap to detect.