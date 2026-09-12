// Package client provides the HTTP client core for generated clients:
// ClientBase, call helpers for unary and streaming RPCs, and WebSocket
// dials. It imports codes, status, encoding, metadata, and runtime, and
// cannot import runtime.Mux (that would be an import cycle).
package client