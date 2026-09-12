# zara-rpc

A Go RPC framework that combines the simplicity of [connect-go](https://connectrpc.com/) with the declarative HTTP endpoints of [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway).

Define your API once in a `.proto` file with `google.api.http` annotations. `protoc-gen-zararpc` generates:

- **Typed handlers** — implement a plain Go interface, no gRPC machinery required.
- **Typed HTTP clients** — call your API over plain HTTP with codec negotiation.
- **REST-to-gRPC gateway adapters** — proxy HTTP requests to an existing gRPC server.

## Features

- **Pure `net/http`** — REST endpoints work over HTTP/1.1 and HTTP/2, no gRPC required on the server side.
- **Declarative routes** — `google.api.http` annotations (path params, query params, body mapping, `additional_bindings`).
- **Streaming transports** — server streaming over SSE or WebSocket, client streaming over NDJSON, bidi streaming over WebSocket.
- **Codec negotiation** — JSON, JSONV2, XML, and Protobuf, selected via `Content-Type` / `Accept` headers.
- **Interceptors** — unary middleware composed like an onion.
- **Request metadata** — handlers can read headers, query params, body, and cookies for auth (JWT) from both HTTP and gRPC callers.
- **gRPC compatibility** — error codes are numerically identical to gRPC status codes; a combined gRPC + HTTP server can share one port via h2c.
- **Gateway** — one-liner `Register*Gateway(mux, conn)` proxies REST to an existing gRPC server, forwarding auth headers.

## Package layout

The layout mirrors `grpc-ecosystem/grpc-gateway`: the framework core lives in `runtime/`, the code generator is a `package main` at the repo root, and public subpackages hold cross-cutting concerns, with functional options (`WithXxx`) for configuration. There are **no root re-exports and no root package** — import the package where you need it:

| Package | Holds |
|---|---|
| `runtime` | `Mux`, `Endpoint`, `Handler`, `Service`, `Request[T]`, streams, interceptors, `Spec`, `ParsePattern`, `PopulateMessage`, `PopulateQuery`, `SubstitutePathParams` |
| `codes` | RPC error code enum (gRPC-compatible) |
| `status` | RPC errors: `NewErrorf`, `Code`, `FromError`, `FromHTTP` |
| `encoding` | `Codec`, `JSONCodec`, `ProtoCodec`, `CodecForRequest` |
| `metadata` | `RequestMeta`, `HeaderFromContext`, `QueryFromContext`, `BodyFromContext`, `CookieFromContext` |
| `peer` | `Peer`, `PeerFromContext` |
| `client` | `ClientBase`, `NewClientBase`, `WithCodec`, `WithHeader`, `WithServerStreamTransport` |
| `server` | `New`, `WithReadTimeout`, `WithTLSConfig`, lifecycle + streaming transports (`NewSSEServerStream`, `NewWSBidiStream`, `AcceptWebSocket`) |
| `auth` | JWT validation + Envoy-style RBAC authorization (grpc-go authz style) |

## Installation

### 1. Install the code generator

```sh
go install github.com/aldok10/zara-rpc/protoc-gen-zararpc@latest
```

This builds the `protoc-gen-zararpc` plugin and installs it to `$(go env GOPATH)/bin`. Make sure that directory is on your `PATH`:

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
```

Verify the plugin is found:

```sh
which protoc-gen-zararpc
```

### 2. Generate with buf

Add the plugin to `buf.gen.yaml`:

```yaml
version: v2
plugins:
  - local: protoc-gen-zararpc
    out: gen
    opt:
      - paths=source_relative
```

Then run:

```sh
buf generate
```

### 3. Generate without buf (plain protoc)

The `google/api/annotations.proto` dependency is vendored in [`third_party/`](third_party/) so generation works offline:

```sh
protoc \
  -I proto \
  -I third_party \
  --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
  --zararpc_out=gen --zararpc_opt=paths=source_relative \
  proto/users/v1/users.proto
```

Notes:

- `--zararpc_out` emits the `.pb.go` message types (a superset of `protoc-gen-go`, with custom struct tags patched in), `.zararpc.go` (handler interface, typed HTTP client, route registration) and `.gateway.go` (REST-to-gRPC gateway adapter). There is no separate `--go_out` step.
- `--go-grpc_out` is only needed when you use the gateway or a gRPC server.
- `paths=source_relative` mirrors the proto directory layout; omit it to get the full module-path layout.

## Quickstart

### 1. Define the API

```proto
syntax = "proto3";
package acme.users.v1;

import "google/api/annotations.proto";

option go_package = "examples/proto/users/v1;usersv1";

service UsersService {
  rpc GetUser(GetUserRequest) returns (User) {
    option (google.api.http) = { get: "/v1/users/{id}" };
  }
  rpc CreateUser(CreateUserRequest) returns (User) {
    option (google.api.http) = { post: "/v1/users", body: "*" };
  }
  rpc WatchUsers(WatchUsersRequest) returns (stream User) {
    option (google.api.http) = { get: "/v1/users:watch" };
  }
}
```

### 2. Generate code

With [buf](https://buf.build) (see [Installation](#installation)):

```sh
buf generate
```

Or with plain protoc:

```sh
protoc \
  -I proto \
  -I third_party \
  --go_out=gen --go_opt=paths=source_relative \
  --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
  --zararpc_out=gen --zararpc_opt=paths=source_relative \
  proto/users/v1/users.proto
```

This produces:

- `service.zararpc.go` — handler interface, typed HTTP client, route registration.
- `service.gateway.go` — REST-to-gRPC gateway adapter (requires `--go-grpc_out`).
- `service.pb.go` / `service_grpc.pb.go` — standard protoc-gen-go output.

### 3. Implement the handler

```go
type usersService struct {
	usersv1.UnimplementedUsersServiceHandler
	users map[string]*usersv1.User
}

func (s *usersService) GetUser(ctx context.Context, req *usersv1.GetUserRequest) (*usersv1.User, error) {
	u, ok := s.users[req.Id]
	if !ok {
		return nil, status.NewErrorf(codes.CodeNotFound, "user %q not found", req.Id)
	}
	return u, nil
}
```

### 4. Register and serve

```go
mux := runtime.NewMux()
if err := usersv1.RegisterUsersServiceRoutes(mux, svc); err != nil {
	log.Fatal(err)
}
http.ListenAndServe(":8080", mux)
```

Or use the built-in server wrapper with sane defaults:

```go
srv := server.New(":8080", mux, server.WithReadTimeout(10*time.Second))
srv.ListenAndServe()
```

### 5. Call from a client

```go
c := usersv1.NewUsersServiceHTTPClient("http://localhost:8080")
u, err := c.GetUser(ctx, &usersv1.GetUserRequest{Id: "1"})
```

## Streaming

| Stream type | HTTP transport | Notes |
|---|---|---|
| Server streaming | SSE (default) or WebSocket | `client.WithServerStreamTransport(client.ServerStreamWebSocket)` |
| Client streaming | NDJSON | one JSON message per line |
| Bidi streaming | WebSocket | `github.com/coder/websocket` |

## Auth & request metadata

Handlers receive a `RequestMeta` via context with everything needed for auth:

```go
func authToken(ctx context.Context) string {
	if h := metadata.HeaderFromContext(ctx).Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if t := metadata.QueryFromContext(ctx).Get("token"); t != "" {
		return t
	}
	if c, err := metadata.CookieFromContext(ctx, "session"); err == nil {
		return c.Value
	}
	return ""
}
```

Accessors live in the `metadata` package: `metadata.HeaderFromContext`, `metadata.QueryFromContext`, `metadata.BodyFromContext`, `metadata.CookiesFromContext`, `metadata.CookieFromContext`.

The same code path works for **gRPC callers** — wrap the context with `metadata.WithRequestMeta` after converting gRPC metadata to an `http.Header` (see `examples/server/main.go` `withGRPCMeta`).

## Gateway (REST → gRPC)

```go
conn, _ := grpc.NewClient("localhost:8080", grpc.WithTransportCredentials(insecure.NewCredentials()))
mux := runtime.NewMux()
usersv1.RegisterUsersServiceGateway(mux, conn) // one-liner
http.ListenAndServe(":8081", mux)
```

The generated gateway forwards request headers (skipping hop-by-hop ones) into gRPC metadata, so `Authorization` survives the hop. gRPC status errors are converted back to zararpc errors, preserving HTTP status codes (401, 404, ...).

## Combined gRPC + HTTP server

Serve both protocols on one port using h2c:

```go
grpcServer := grpc.NewServer(grpc.UnaryInterceptor(unaryInterceptor))
usersv1.RegisterUsersServiceServer(grpcServer, svc)
reflection.Register(grpcServer)

mux := runtime.NewMux()
usersv1.RegisterUsersServiceRoutes(mux, svc)

handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
		grpcServer.ServeHTTP(w, r)
		return
	}
	mux.ServeHTTP(w, r)
})
http.ListenAndServe(":8080", h2c.NewHandler(handler, &http2.Server{}))
```

## Error codes

`codes.Code` is numerically identical to gRPC `codes.Code` (0–16), so conversions between the two are direct casts. Errors serialize to JSON:

```json
{"code": "not_found", "message": "user \"1\" not found"}
```

## Example

A complete `UsersService` lives in [`examples/`](examples/) with all four stream types, auth, a gateway, and a combined server. The examples are a **git submodule** pointing at [`github.com/aldok10/zara-rpc-examples`](https://github.com/aldok10/zara-rpc-examples) — a separate Go module (`github.com/aldok10/zara-rpc-examples`) that depends on the framework via a `replace` directive, so the framework itself stays free of gRPC dependencies.

Clone with submodules, or initialize them in an existing checkout:

```sh
git clone --recurse-submodules https://github.com/aldok10/zara-rpc
# or: git submodule update --init --recursive
```

Then run the example stack:

```sh
cd examples

# server (gRPC + HTTP on :8080)
go run ./server

# gateway (REST → gRPC on :8081)
go run ./gateway

# client demo (JSON, Protobuf, SSE, NDJSON, WebSocket)
go run ./client
```

## Development

The repository is two Go modules: the framework root (grpc-free) and `examples/` (gRPC + gateway demo, a git submodule).

```sh
go build ./...     # build the framework + generator (root module)
go test ./...      # run framework tests (root module)
cd examples && go build ./...   # build the examples module
```

The code generator is `protoc-gen-zararpc` (a `package main` at the repo root, mirroring grpc-gateway's `protoc-gen-grpc-gateway`); rebuild it after changes and regenerate:

```sh
go build -o "$(go env GOPATH)/bin/protoc-gen-zararpc" ./protoc-gen-zararpc
cd examples && buf generate
```

`google/api/annotations.proto` and `http.proto` are vendored in [`third_party/`](third_party/) for offline generation.

## License

MIT