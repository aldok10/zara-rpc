package jwt

import (
	"encoding/json"

	"github.com/golang-jwt/jwt/v5"

	"github.com/aldok10/zara-rpc/runtime"
)

// claimsKey is the context key under which validated claims are stored.
type claimsKey struct{}

// Claims is the validated JWT payload. RegisteredClaims carries the
// standard exp/nbf/iat/iss/aud/sub claims; Custom holds any other claims
// from the token (e.g. "role").
type Claims struct {
	jwt.RegisteredClaims
	Custom map[string]any
}

// UnmarshalJSON parses the standard claims into RegisteredClaims and
// captures every other claim in Custom.
func (c *Claims) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &c.RegisteredClaims); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Custom = make(map[string]any)
	for k, v := range raw {
		if isRegisteredClaim(k) {
			continue
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			return err
		}
		c.Custom[k] = val
	}
	return nil
}

func isRegisteredClaim(name string) bool {
	switch name {
	case "exp", "nbf", "iat", "iss", "aud", "sub", "jti":
		return true
	}
	return false
}

// ClaimsFromContext returns the validated claims attached by the JWT
// interceptor, or false when the request was not authenticated.
func ClaimsFromContext(ctx runtime.Ctx) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*Claims)
	return c, ok
}

// WithClaims returns a Ctx carrying the validated claims, so downstream
// interceptors and handlers can read them via ClaimsFromContext.
func WithClaims(ctx runtime.Ctx, claims *Claims) runtime.Ctx {
	return ctx.WithValue(claimsKey{}, claims)
}

// HasRole reports whether the "role" claim contains the given value. The
// role claim may be a string or an array of strings.
func (c *Claims) HasRole(role string) bool {
	for _, r := range c.Roles() {
		if r == role {
			return true
		}
	}
	return false
}

// Roles returns the "role" claim as a slice, normalizing a single string
// or an array of strings.
func (c *Claims) Roles() []string {
	v, ok := c.Custom["role"]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		roles := make([]string, 0, len(t))
		for _, r := range t {
			if s, ok := r.(string); ok {
				roles = append(roles, s)
			}
		}
		return roles
	default:
		return nil
	}
}
