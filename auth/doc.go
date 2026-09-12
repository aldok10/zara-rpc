// Package auth provides authentication and authorization for zara-rpc,
// organized into focused subpackages:
//
//   - auth/jwt: JWT token validation, claims parsing, and token issuance.
//   - auth/rbac: Envoy-style RBAC policy evaluation (deny-first, allow/deny rules).
//   - auth/interceptor: Composes JWT validation and RBAC authorization into
//     the runtime unary/stream interceptor chain.
//
// The package is grpc-free: interceptors compose as runtime.UnaryInterceptor
// (runtime.WithMuxUnaryInterceptors) and read transport payloads from
// runtime.Ctx, so one policy protects HTTP, WebSocket, and gRPC uniformly.
package auth
