package rbac

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/aldok10/zara-rpc/auth/jwt"
	"github.com/aldok10/zara-rpc/runtime"
)

// Policy is an Envoy-style RBAC authorization policy, the same shape
// grpc-go's authz package consumes. Evaluation is deny-first: if any deny
// rule matches, the request is rejected; otherwise the request is allowed
// only when at least one allow rule matches. An empty allow list denies
// everything.
//
// Example:
//
//	{
//	  "name": "users-policy",
//	  "allow_rules": [
//	    {"name": "admins", "principals": [{"authenticated": {"claim": "role", "value": "admin"}}], "permissions": [{"any": true}]},
//	    {"name": "readers", "principals": [{"any": true}], "permissions": [{"requested_path": {"prefix": "/acme.users.v1.UsersService/Get"}}]}
//	  ],
//	  "deny_rules": [
//	    {"name": "no-delete", "principals": [{"any": true}], "permissions": [{"requested_path": {"exact": "/acme.users.v1.UsersService/DeleteUser"}}]}
//	  ]
//	}
type Policy struct {
	Name       string `json:"name"`
	AllowRules []Rule `json:"allow_rules"`
	DenyRules  []Rule `json:"deny_rules"`
}

// Rule pairs a set of principals (who) with a set of permissions (what).
// A rule matches when any principal AND any permission match.
type Rule struct {
	Name        string       `json:"name"`
	Principals  []Principal  `json:"principals"`
	Permissions []Permission `json:"permissions"`
}

// Principal identifies a caller. Exactly one field is set.
type Principal struct {
	// Authenticated matches the JWT claims attached by the auth
	// unary interceptor. principal_name matches the "sub" claim; claim/value
	// matches a custom claim (e.g. role).
	Authenticated *Authenticated `json:"authenticated"`
	// SourceIP matches the client address against a CIDR block.
	SourceIP *CIDR `json:"source_ip"`
	// Header matches a request header (case-insensitive name, exact value).
	Header *HeaderMatch `json:"header"`
	// Any matches every caller.
	Any bool `json:"any"`
	// Not inverts a nested principal.
	Not *Principal `json:"not"`
	// AndIds matches when every nested principal matches.
	AndIds []Principal `json:"and_ids"`
	// OrIds matches when any nested principal matches.
	OrIds []Principal `json:"or_ids"`
}

// Authenticated matches the validated JWT claims.
type Authenticated struct {
	// PrincipalName matches the "sub" claim exactly.
	PrincipalName string `json:"principal_name"`
	// Claim/Value matches a custom claim exactly (e.g. role=admin).
	Claim string `json:"claim"`
	Value string `json:"value"`
}

// CIDR matches a client address against a CIDR block.
type CIDR struct {
	Address string `json:"address"`
}

// HeaderMatch matches a request header.
type HeaderMatch struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Permission describes what a rule may do. Exactly one field is set.
type Permission struct {
	// RequestedPath matches the RPC procedure, e.g.
	// "/acme.users.v1.UsersService/GetUser".
	RequestedPath *PathMatch `json:"requested_path"`
	// Header matches a request header.
	Header *HeaderMatch `json:"header"`
	// Any matches every request.
	Any bool `json:"any"`
	// Not inverts a nested permission.
	Not *Permission `json:"not"`
	// AndPermissions matches when every nested permission matches.
	AndPermissions []Permission `json:"and_permissions"`
	// OrPermissions matches when any nested permission matches.
	OrPermissions []Permission `json:"or_permissions"`
}

// PathMatch matches the RPC procedure.
type PathMatch struct {
	Exact  string `json:"exact"`
	Prefix string `json:"prefix"`
}

func parsePolicy(data []byte) (*Policy, error) {
	p := &Policy{}
	if err := json.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("authz: parse policy: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("authz: policy missing name")
	}
	return p, nil
}

func (p *Principal) matches(ctx runtime.Ctx) bool {
	switch {
	case p.Authenticated != nil:
		claims, ok := jwt.ClaimsFromContext(ctx)
		if !ok {
			return false
		}
		if p.Authenticated.PrincipalName != "" {
			return claims.Subject == p.Authenticated.PrincipalName
		}
		if p.Authenticated.Claim != "" {
			return claimEquals(claims, p.Authenticated.Claim, p.Authenticated.Value)
		}
		return false
	case p.SourceIP != nil:
		_, cidr, err := net.ParseCIDR(p.SourceIP.Address)
		if err != nil {
			return false
		}
		host, _, err := net.SplitHostPort(ctx.Peer().Addr)
		if err != nil {
			return false
		}
		ip := net.ParseIP(host)
		return ip != nil && cidr.Contains(ip)
	case p.Header != nil:
		return headerEquals(ctx, p.Header.Name, p.Header.Value)
	case p.Any:
		return true
	case p.Not != nil:
		return !p.Not.matches(ctx)
	case len(p.AndIds) > 0:
		for i := range p.AndIds {
			if !p.AndIds[i].matches(ctx) {
				return false
			}
		}
		return true
	case len(p.OrIds) > 0:
		for i := range p.OrIds {
			if p.OrIds[i].matches(ctx) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (p *Permission) matches(ctx runtime.Ctx) bool {
	switch {
	case p.RequestedPath != nil:
		path := ctx.Spec().Procedure
		if p.RequestedPath.Exact != "" {
			return path == p.RequestedPath.Exact
		}
		if p.RequestedPath.Prefix != "" {
			return strings.HasPrefix(path, p.RequestedPath.Prefix)
		}
		return false
	case p.Header != nil:
		return headerEquals(ctx, p.Header.Name, p.Header.Value)
	case p.Any:
		return true
	case p.Not != nil:
		return !p.Not.matches(ctx)
	case len(p.AndPermissions) > 0:
		for i := range p.AndPermissions {
			if !p.AndPermissions[i].matches(ctx) {
				return false
			}
		}
		return true
	case len(p.OrPermissions) > 0:
		for i := range p.OrPermissions {
			if p.OrPermissions[i].matches(ctx) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// ruleMatches reports whether any principal and any permission match.
func (r *Rule) matches(ctx runtime.Ctx) bool {
	if len(r.Principals) == 0 || len(r.Permissions) == 0 {
		return false
	}
	principal := false
	for i := range r.Principals {
		if r.Principals[i].matches(ctx) {
			principal = true
			break
		}
	}
	if !principal {
		return false
	}
	for i := range r.Permissions {
		if r.Permissions[i].matches(ctx) {
			return true
		}
	}
	return false
}

// isAuthorized evaluates the policy: deny rules first, then allow rules.
func (p *Policy) isAuthorized(ctx runtime.Ctx) bool {
	for i := range p.DenyRules {
		if p.DenyRules[i].matches(ctx) {
			return false
		}
	}
	for i := range p.AllowRules {
		if p.AllowRules[i].matches(ctx) {
			return true
		}
	}
	return false
}

func claimEquals(claims *jwt.Claims, name, want string) bool {
	v, ok := claims.Custom[name]
	if !ok {
		return false
	}
	switch t := v.(type) {
	case string:
		return t == want
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

// headerEquals compares a request header case-insensitively by name and
// exactly by value.
func headerEquals(ctx runtime.Ctx, name, want string) bool {
	for k, vs := range ctx.Header() {
		if !strings.EqualFold(k, name) {
			continue
		}
		for _, v := range vs {
			if v == want {
				return true
			}
		}
	}
	return false
}
