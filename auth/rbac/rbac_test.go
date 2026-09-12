package rbac

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	zajwt "github.com/aldok10/zara-rpc/auth/jwt"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/runtime"
)

const testPolicy = `{
  "name": "users-policy",
  "allow_rules": [
    {"name": "admins", "principals": [{"authenticated": {"claim": "role", "value": "admin"}}], "permissions": [{"any": true}]},
    {"name": "readers", "principals": [{"any": true}], "permissions": [{"requested_path": {"prefix": "/acme.users.v1.UsersService/Get"}}]}
  ],
  "deny_rules": [
    {"name": "no-delete", "principals": [{"any": true}], "permissions": [{"requested_path": {"exact": "/acme.users.v1.UsersService/DeleteUser"}}]}
  ]
}`

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

// authedCtx builds a Ctx with validated claims attached.
func authedCtx(procedure string, custom map[string]any) runtime.Ctx {
	ctx := testCtx(nil, procedure)
	claims := &zajwt.Claims{Custom: custom}
	return zajwt.WithClaims(ctx, claims)
}

func TestStaticAuthorizer(t *testing.T) {
	a, err := NewStatic(testPolicy)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("admin allowed on any non-denied path", func(t *testing.T) {
		ctx := authedCtx("/acme.users.v1.UsersService/CreateUser", map[string]any{"role": "admin"})
		if !a.IsAuthorized(ctx) {
			t.Fatalf("IsAuthorized = false, want true")
		}
	})

	t.Run("anyone allowed on read paths", func(t *testing.T) {
		ctx := authedCtx("/acme.users.v1.UsersService/GetUser", map[string]any{"role": "user"})
		if !a.IsAuthorized(ctx) {
			t.Fatalf("IsAuthorized = false, want true")
		}
	})

	t.Run("deny rule beats allow rule", func(t *testing.T) {
		// DeleteUser matches the deny rule even for admins; deny rules
		// are evaluated first and take precedence.
		ctx := authedCtx("/acme.users.v1.UsersService/DeleteUser", map[string]any{"role": "admin"})
		if a.IsAuthorized(ctx) {
			t.Fatalf("IsAuthorized = true, want false")
		}
	})

	t.Run("unauthenticated denied on non-public path", func(t *testing.T) {
		// The readers rule allows anyone on Get paths, so a public read
		// is fine; a write path requires an authenticated admin.
		ctx := testCtx(nil, "/acme.users.v1.UsersService/CreateUser")
		if a.IsAuthorized(ctx) {
			t.Fatalf("IsAuthorized = true, want false")
		}
	})

	t.Run("unknown path denied", func(t *testing.T) {
		ctx := authedCtx("/acme.users.v1.UsersService/CreateUser", map[string]any{"role": "user"})
		if a.IsAuthorized(ctx) {
			t.Fatalf("IsAuthorized = true, want false")
		}
	})
}

func TestPrincipalMatching(t *testing.T) {
	t.Run("subject match", func(t *testing.T) {
		p := &Principal{Authenticated: &Authenticated{PrincipalName: "users/1"}}
		ctx := authedCtx("", nil)
		ctx = zajwt.WithClaims(ctx, &zajwt.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "users/1"}})
		if !p.matches(ctx) {
			t.Error("subject match = false, want true")
		}
	})

	t.Run("header match", func(t *testing.T) {
		p := &Principal{Header: &HeaderMatch{Name: "X-Tenant", Value: "acme"}}
		ctx := testCtx(map[string]string{"X-Tenant": "acme"}, "")
		if !p.matches(ctx) {
			t.Error("header match = false, want true")
		}
	})

	t.Run("not inverts", func(t *testing.T) {
		p := &Principal{Not: &Principal{Any: true}}
		ctx := authedCtx("", nil)
		if p.matches(ctx) {
			t.Error("not(any) = true, want false")
		}
	})

	t.Run("and requires all", func(t *testing.T) {
		p := &Principal{AndIds: []Principal{
			{Header: &HeaderMatch{Name: "X-Tenant", Value: "acme"}},
			{Header: &HeaderMatch{Name: "X-Region", Value: "us"}},
		}}
		ctx := testCtx(map[string]string{"X-Tenant": "acme", "X-Region": "eu"}, "")
		if p.matches(ctx) {
			t.Error("and(acme, eu) = true, want false")
		}
	})

	t.Run("or matches any", func(t *testing.T) {
		p := &Principal{OrIds: []Principal{
			{Header: &HeaderMatch{Name: "X-Tenant", Value: "acme"}},
			{Header: &HeaderMatch{Name: "X-Tenant", Value: "globex"}},
		}}
		ctx := testCtx(map[string]string{"X-Tenant": "globex"}, "")
		if !p.matches(ctx) {
			t.Error("or(acme, globex) = false, want true")
		}
	})
}

func TestPermissionMatching(t *testing.T) {
	t.Run("path exact", func(t *testing.T) {
		p := &Permission{RequestedPath: &PathMatch{Exact: "/svc/Method"}}
		ctx := testCtx(nil, "/svc/Method")
		if !p.matches(ctx) {
			t.Error("exact match = false, want true")
		}
	})

	t.Run("path prefix", func(t *testing.T) {
		p := &Permission{RequestedPath: &PathMatch{Prefix: "/svc/Get"}}
		ctx := testCtx(nil, "/svc/GetUser")
		if !p.matches(ctx) {
			t.Error("prefix match = false, want true")
		}
	})

	t.Run("not inverts", func(t *testing.T) {
		p := &Permission{Not: &Permission{Any: true}}
		ctx := testCtx(nil, "/svc/Method")
		if p.matches(ctx) {
			t.Error("not(any) = true, want false")
		}
	})
}

func TestFileWatcherReload(t *testing.T) {
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(policyFile, []byte(testPolicy), 0o644); err != nil {
		t.Fatal(err)
	}

	updates := make(chan string, 1)
	a, err := NewFileWatcherWithOptions(FileWatcherOptions{
		PolicyFile:      policyFile,
		RefreshDuration: 20 * time.Millisecond,
		OnPolicyUpdate:  func(p string) { updates <- p },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	// The constructor fires OnPolicyUpdate with the initial policy; drain
	// it so the reload below is the update we observe.
	select {
	case <-updates:
	case <-time.After(2 * time.Second):
		t.Fatal("initial policy update not observed")
	}

	// Initial policy: admin allowed on CreateUser.
	ctx := authedCtx("/acme.users.v1.UsersService/CreateUser", map[string]any{"role": "admin"})
	if !a.IsAuthorized(ctx) {
		t.Fatalf("initial policy: IsAuthorized = false, want true")
	}

	// Rewrite the policy to deny admins on CreateUser.
	denyAll := `{
	  "name": "deny-all",
	  "allow_rules": [
	    {"name": "admins", "principals": [{"authenticated": {"claim": "role", "value": "admin"}}], "permissions": [{"requested_path": {"prefix": "/acme.users.v1.UsersService/Get"}}]}
	  ]
	}`
	if err := os.WriteFile(policyFile, []byte(denyAll), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-updates:
	case <-time.After(2 * time.Second):
		t.Fatal("policy update not observed")
	}

	// Now the same admin request is denied.
	if a.IsAuthorized(ctx) {
		t.Fatalf("after reload: IsAuthorized = true, want false")
	}
}

func TestParsePolicyErrors(t *testing.T) {
	if _, err := NewStatic(`{"name": ""}`); err == nil {
		t.Error("NewStatic(empty name) = nil, want error")
	}
	if _, err := NewStatic(`not json`); err == nil {
		t.Error("NewStatic(bad json) = nil, want error")
	}
}
