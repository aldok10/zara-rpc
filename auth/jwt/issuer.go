package jwt

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer signs HS256 JWTs for the example's auth flow.
type Issuer struct {
	key    []byte
	method jwt.SigningMethod
	issuer string
}

// IssuerOption configures an Issuer.
type IssuerOption func(*Issuer)

// WithIssuerName sets the "iss" claim on issued tokens.
func WithIssuerName(name string) IssuerOption {
	return func(i *Issuer) { i.issuer = name }
}

// NewIssuer creates an HS256 token issuer. key is the HMAC secret.
func NewIssuer(key []byte, opts ...IssuerOption) *Issuer {
	i := &Issuer{key: key, method: jwt.SigningMethodHS256}
	for _, opt := range opts {
		opt(i)
	}
	return i
}

// Issue signs a JWT for the given subject with custom claims (e.g. "role").
// The token expires after ttl.
func (i *Issuer) Issue(subject string, custom map[string]any, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": subject,
		"exp": now.Add(ttl).Unix(),
		"iat": now.Unix(),
	}
	if i.issuer != "" {
		claims["iss"] = i.issuer
	}
	for k, v := range custom {
		claims[k] = v
	}
	token := jwt.NewWithClaims(i.method, claims)
	return token.SignedString(i.key)
}
