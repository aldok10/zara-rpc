package jwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/runtime"
)

const testSecret = "test-secret-key"

// signToken mints an HS256 token with the test secret.
func signToken(claims jwt.Claims) string {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := t.SignedString([]byte(testSecret))
	if err != nil {
		panic(err)
	}
	return s
}

// testCtx builds a runtime.Ctx with the given headers and spec.
func testCtx(headers map[string]string, procedure string) runtime.Ctx {
	h := make(map[string][]string)
	for k, v := range headers {
		h[k] = []string{v}
	}
	meta := metadata.RequestMeta{Header: h}
	ctx := runtime.NewCtx(context.Background(), meta, encoding.JSONCodec{})
	if procedure != "" {
		ctx = ctx.WithSpec(runtime.Spec{Procedure: procedure})
	}
	return ctx
}

func TestValidateValidToken(t *testing.T) {
	v, err := NewJWTValidator([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	token := signToken(jwt.MapClaims{
		"sub":  "users/1",
		"role": "admin",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	claims, err := v.Validate(token)
	if err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
	if claims.Subject != "users/1" {
		t.Errorf("Subject = %q, want users/1", claims.Subject)
	}
	if !claims.HasRole("admin") {
		t.Errorf("HasRole(admin) = false, want true")
	}
}

func TestValidateExpiredToken(t *testing.T) {
	v, _ := NewJWTValidator([]byte(testSecret))
	token := signToken(jwt.MapClaims{
		"sub": "users/1",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	if _, err := v.Validate(token); err == nil {
		t.Fatal("Validate(expired) = nil, want error")
	}
}

func TestValidateTamperedToken(t *testing.T) {
	v, _ := NewJWTValidator([]byte(testSecret))
	token := signToken(jwt.MapClaims{"sub": "users/1", "exp": time.Now().Add(time.Hour).Unix()})
	tampered := token[:len(token)-2] + "xx"
	if _, err := v.Validate(tampered); err == nil {
		t.Fatal("Validate(tampered) = nil, want error")
	}
}

func TestValidateWrongSecret(t *testing.T) {
	v, _ := NewJWTValidator([]byte("other-secret"))
	token := signToken(jwt.MapClaims{"sub": "users/1", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := v.Validate(token); err == nil {
		t.Fatal("Validate(wrong secret) = nil, want error")
	}
}

func TestValidateAlgConfusion(t *testing.T) {
	// A token signed with HS256 must not be accepted when the validator
	// expects RS256 (alg-confusion attack).
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := NewJWTValidatorWithKey(&key.PublicKey)
	token := signToken(jwt.MapClaims{"sub": "users/1", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := v.Validate(token); err == nil {
		t.Fatal("Validate(HS256 into RS256 validator) = nil, want error")
	}
}

func TestValidateIssuerAudience(t *testing.T) {
	v, _ := NewJWTValidator([]byte(testSecret), WithIssuer("zara"), WithAudience("users-api"))
	token := signToken(jwt.MapClaims{
		"sub": "users/1",
		"iss": "zara",
		"aud": "users-api",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Validate(token); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}

	badIss := signToken(jwt.MapClaims{"sub": "users/1", "iss": "evil", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := v.Validate(badIss); err == nil {
		t.Fatal("Validate(wrong issuer) = nil, want error")
	}

	badAud := signToken(jwt.MapClaims{"sub": "users/1", "aud": "other-api", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := v.Validate(badAud); err == nil {
		t.Fatal("Validate(wrong audience) = nil, want error")
	}
}

func TestDefaultTokenExtractor(t *testing.T) {
	token := "abc.def.ghi"
	ctx := testCtx(map[string]string{"Authorization": "Bearer " + token}, "")
	got, ok := DefaultTokenExtractor(ctx)
	if !ok || got != token {
		t.Errorf("DefaultTokenExtractor = %q, %v; want %q, true", got, ok, token)
	}

	ctx2 := testCtx(map[string]string{"Authorization": "Basic abc"}, "")
	if _, ok := DefaultTokenExtractor(ctx2); ok {
		t.Error("DefaultTokenExtractor(Basic) = true, want false")
	}
}

func TestExtractToken(t *testing.T) {
	v, err := NewJWTValidator([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	token := "my-test-token"
	ctx := testCtx(map[string]string{"Authorization": "Bearer " + token}, "")
	got, ok := v.ExtractToken(ctx)
	if !ok || got != token {
		t.Errorf("ExtractToken = %q, %v; want %q, true", got, ok, token)
	}

	empty := testCtx(nil, "")
	if _, ok := v.ExtractToken(empty); ok {
		t.Error("ExtractToken(none) = true, want false")
	}
}

func TestWithClaimsAndClaimsFromContext(t *testing.T) {
	ctx := testCtx(nil, "")
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "users/1"},
		Custom:           map[string]any{"role": "admin"},
	}
	ctx = WithClaims(ctx, claims)

	got, ok := ClaimsFromContext(ctx)
	if !ok {
		t.Fatal("ClaimsFromContext = false, want true")
	}
	if got.Subject != "users/1" {
		t.Errorf("Subject = %q, want users/1", got.Subject)
	}

	// An unauthenticated context returns false.
	empty := testCtx(nil, "")
	if _, ok := ClaimsFromContext(empty); ok {
		t.Error("ClaimsFromContext(unauthenticated) = true, want false")
	}
}

func TestClaimsRoles(t *testing.T) {
	t.Run("single string role", func(t *testing.T) {
		c := &Claims{Custom: map[string]any{"role": "admin"}}
		if !c.HasRole("admin") {
			t.Error("HasRole(admin) = false, want true")
		}
		if c.HasRole("user") {
			t.Error("HasRole(user) = true, want false")
		}
		roles := c.Roles()
		if len(roles) != 1 || roles[0] != "admin" {
			t.Errorf("Roles = %v, want [admin]", roles)
		}
	})

	t.Run("array of roles", func(t *testing.T) {
		c := &Claims{Custom: map[string]any{"role": []any{"admin", "user"}}}
		if !c.HasRole("user") {
			t.Error("HasRole(user) = false, want true")
		}
		roles := c.Roles()
		if len(roles) != 2 {
			t.Errorf("Roles = %v, want [admin user]", roles)
		}
	})

	t.Run("missing role", func(t *testing.T) {
		c := &Claims{}
		if c.HasRole("admin") {
			t.Error("HasRole(admin) = true, want false")
		}
		if c.Roles() != nil {
			t.Errorf("Roles = %v, want nil", c.Roles())
		}
	})
}
