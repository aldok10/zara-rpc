package jwt

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/aldok10/zara-rpc/runtime"
)

// TokenExtractor pulls a bearer token out of a request. The default
// extractor reads the Authorization header ("Bearer <token>"), then the
// "token" query parameter, then the "session" cookie — the same order the
// example handlers use. Custom extractors can be supplied with
// WithTokenExtractor.
type TokenExtractor func(ctx runtime.Ctx) (token string, ok bool)

// DefaultTokenExtractor reads the Authorization header first, then the
// "token" query parameter, then the "session" cookie.
func DefaultTokenExtractor(ctx runtime.Ctx) (string, bool) {
	if h := ctx.Header().Get("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
		return h[7:], true
	}
	if t := ctx.Query().Get("token"); t != "" {
		return t, true
	}
	if c, err := ctx.Cookie("session"); err == nil && c.Value != "" {
		return c.Value, true
	}
	return "", false
}

// JWTValidator validates signed JWTs. HS256 is the default (shared
// secret); pass an *rsa.PublicKey or *ecdsa.PublicKey to NewJWTValidator
// to use RS256/ES256 instead.
type JWTValidator struct {
	key      any
	method   jwt.SigningMethod
	issuer   string
	audience string
	leeway   time.Duration
	extract  TokenExtractor
}

// JWTOption configures a JWTValidator.
type JWTOption func(*JWTValidator)

// WithIssuer requires the token's "iss" claim to match.
func WithIssuer(iss string) JWTOption {
	return func(v *JWTValidator) { v.issuer = iss }
}

// WithAudience requires the token's "aud" claim to contain the value.
func WithAudience(aud string) JWTOption {
	return func(v *JWTValidator) { v.audience = aud }
}

// WithLeeway relaxes the exp/nbf clock-skew window.
func WithLeeway(d time.Duration) JWTOption {
	return func(v *JWTValidator) { v.leeway = d }
}

// WithTokenExtractor replaces the default token extraction strategy.
func WithTokenExtractor(fn TokenExtractor) JWTOption {
	return func(v *JWTValidator) { v.extract = fn }
}

// NewJWTValidator returns a validator for HS256 tokens signed with secret.
func NewJWTValidator(secret []byte, opts ...JWTOption) (*JWTValidator, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("auth: empty HMAC secret")
	}
	v := &JWTValidator{
		key:     secret,
		method:  jwt.SigningMethodHS256,
		extract: DefaultTokenExtractor,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v, nil
}

// NewJWTValidatorWithKey returns a validator for asymmetric tokens. Pass an
// *rsa.PublicKey for RS256 or an *ecdsa.PublicKey for ES256.
func NewJWTValidatorWithKey(key any, opts ...JWTOption) (*JWTValidator, error) {
	var method jwt.SigningMethod
	switch key.(type) {
	case *rsa.PublicKey:
		method = jwt.SigningMethodRS256
	case *ecdsa.PublicKey:
		method = jwt.SigningMethodES256
	default:
		return nil, fmt.Errorf("auth: unsupported key type %T (want *rsa.PublicKey or *ecdsa.PublicKey)", key)
	}
	v := &JWTValidator{
		key:     key,
		method:  method,
		extract: DefaultTokenExtractor,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v, nil
}

// Validate parses and verifies a token, returning its claims. It rejects
// tokens with an invalid signature, the wrong algorithm (alg-confusion
// protection), an expired exp, a not-yet-valid nbf, or a mismatched
// iss/aud when those options are set.
func (v *JWTValidator) Validate(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method != v.method {
			return nil, fmt.Errorf("auth: unexpected signing method %v", t.Method.Alg())
		}
		return v.key, nil
	}, jwt.WithValidMethods([]string{v.method.Alg()}), jwt.WithLeeway(v.leeway))
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("auth: invalid token")
	}
	if v.issuer != "" && claims.Issuer != v.issuer {
		return nil, fmt.Errorf("auth: issuer %q does not match %q", claims.Issuer, v.issuer)
	}
	if v.audience != "" && !audienceContains(claims.Audience, v.audience) {
		return nil, fmt.Errorf("auth: audience %v does not contain %q", claims.Audience, v.audience)
	}
	return claims, nil
}

// ExtractToken pulls the bearer token out of the request using the
// configured extractor.
func (v *JWTValidator) ExtractToken(ctx runtime.Ctx) (string, bool) {
	return v.extract(ctx)
}

func audienceContains(aud jwt.ClaimStrings, want string) bool {
	for _, a := range aud {
		if a == want {
			return true
		}
	}
	return false
}
