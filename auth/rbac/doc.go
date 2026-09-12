// Package rbac provides Envoy-style RBAC authorization for the zara-rpc
// authorization layer. Policies are deny-first: if any deny rule matches,
// the request is rejected; otherwise the request is allowed only when at
// least one allow rule matches.
package rbac
