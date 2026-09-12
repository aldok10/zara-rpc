// Package codes defines RPC error codes, compatible with gRPC status
// codes (numeric values 0-16). It mirrors google.golang.org/grpc/codes
// and is the leaf dependency of zara-rpc: status, runtime, client, and
// server import it, and it imports nothing from the framework.
package codes