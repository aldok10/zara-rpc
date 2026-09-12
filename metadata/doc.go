// Package metadata carries the transport-level request data available to
// handlers: RequestMeta (headers, query, body, cookies, params), HTTP
// header name/value constants, and HeaderPairs. It mirrors
// google.golang.org/grpc/metadata and is imported by runtime, client,
// and the gateway.
package metadata