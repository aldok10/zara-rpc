package jwt

import (
	"testing"
	"time"
)

func TestIssuerIssueAndValidate(t *testing.T) {
	issuer := NewIssuer([]byte("test-secret"))
	token, err := issuer.Issue("user1", map[string]any{"role": "admin"}, time.Hour)
	if err != nil {
		t.Fatalf("Issue = %v, want nil", err)
	}

	validator, err := NewJWTValidator([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := validator.Validate(token)
	if err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
	if claims.Subject != "user1" {
		t.Errorf("Subject = %q, want user1", claims.Subject)
	}
	if !claims.HasRole("admin") {
		t.Errorf("HasRole(admin) = false, want true")
	}
}
