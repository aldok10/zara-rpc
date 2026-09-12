// Package status provides RPC errors built on codes: NewErrorf, Code,
// FromError, FromHTTP, and FromGRPCStatus. It mirrors
// google.golang.org/grpc/status and is imported by runtime, client,
// server, and auth.
package status