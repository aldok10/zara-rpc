package interceptor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	zajwt "github.com/aldok10/zara-rpc/auth/jwt"
	"github.com/aldok10/zara-rpc/auth/rbac"
	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/runtime"
	"github.com/aldok10/zara-rpc/status"
)

const testSecret = "test-secret-key"

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

// authedCtx builds a Ctx with validated claims attached.
func authedCtx(procedure string, custom map[string]any) runtime.Ctx {
	ctx := testCtx(nil, procedure)
	claims := &zajwt.Claims{Custom: custom}
	return zajwt.WithClaims(ctx, claims)
}

// fakeStream is a minimal runtime.Stream for stream interceptor tests.
type fakeStream struct {
	ctx runtime.Ctx
}

func (f *fakeStream) Context() runtime.Ctx { return f.ctx }
func (f *fakeStream) Send(any) error       { return nil }
func (f *fakeStream) Receive() (any, error) {
	return nil, context.Canceled
}

// streamCtx builds a Ctx with the given headers.
func streamCtx(headers map[string]string) runtime.Ctx {
	h := make(map[string][]string)
	for k, v := range headers {
		h[k] = []string{v}
	}
	return runtime.NewCtx(context.Background(), metadata.RequestMeta{Header: h}, encoding.JSONCodec{})
}

func TestJWTUnaryInterceptor(t *testing.T) {
	v, err := zajwt.NewJWTValidator([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	interceptor := JWT(v)

	next := func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
		claims, ok := zajwt.ClaimsFromContext(ctx)
		if !ok {
			t.Error("ClaimsFromContext = false, want true")
		}
		if claims.Subject != "users/1" {
			t.Errorf("Subject = %q, want users/1", claims.Subject)
		}
		return nil, nil
	}
	wrapped := interceptor.WrapUnary(next)

	t.Run("valid token attaches claims", func(t *testing.T) {
		token := signToken(jwt.MapClaims{"sub": "users/1", "exp": time.Now().Add(time.Hour).Unix()})
		ctx := testCtx(map[string]string{"Authorization": "Bearer " + token}, "")
		if _, err := wrapped(ctx, nil); err != nil {
			t.Fatalf("wrapped = %v, want nil", err)
		}
	})

	t.Run("missing token rejected", func(t *testing.T) {
		ctx := testCtx(nil, "")
		_, err := wrapped(ctx, nil)
		if status.Code(err) != codes.CodeUnauthenticated {
			t.Fatalf("Code = %v, want unauthenticated", status.Code(err))
		}
	})

	t.Run("invalid token rejected", func(t *testing.T) {
		ctx := testCtx(map[string]string{"Authorization": "Bearer garbage"}, "")
		_, err := wrapped(ctx, nil)
		if status.Code(err) != codes.CodeUnauthenticated {
			t.Fatalf("Code = %v, want unauthenticated", status.Code(err))
		}
	})
}

func TestJWTStreamInterceptor(t *testing.T) {
	v, err := zajwt.NewJWTValidator([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	interceptor := JWTStream(v)

	t.Run("valid token attaches claims to stream context", func(t *testing.T) {
		token := signToken(jwt.MapClaims{
			"sub":  "users/1",
			"role": "admin",
			"exp":  time.Now().Add(time.Hour).Unix(),
		})
		ctx := streamCtx(map[string]string{"Authorization": "Bearer " + token})
		stream := interceptor.WrapStream(&fakeStream{ctx: ctx})

		claims, ok := zajwt.ClaimsFromContext(stream.Context())
		if !ok {
			t.Fatal("ClaimsFromContext = false, want true")
		}
		if claims.Subject != "users/1" {
			t.Errorf("Subject = %q, want users/1", claims.Subject)
		}
		if !claims.HasRole("admin") {
			t.Errorf("HasRole(admin) = false, want true")
		}
	})

	t.Run("missing token returns error stream", func(t *testing.T) {
		stream := interceptor.WrapStream(&fakeStream{ctx: streamCtx(nil)})
		if _, err := stream.Receive(); status.Code(err) != codes.CodeUnauthenticated {
			t.Fatalf("Receive Code = %v, want unauthenticated", status.Code(err))
		}
		if err := stream.Send(nil); status.Code(err) != codes.CodeUnauthenticated {
			t.Fatalf("Send Code = %v, want unauthenticated", status.Code(err))
		}
	})

	t.Run("invalid token returns error stream", func(t *testing.T) {
		ctx := streamCtx(map[string]string{"Authorization": "Bearer not-a-jwt"})
		stream := interceptor.WrapStream(&fakeStream{ctx: ctx})
		if _, err := stream.Receive(); status.Code(err) != codes.CodeUnauthenticated {
			t.Fatalf("Receive Code = %v, want unauthenticated", status.Code(err))
		}
	})
}

func TestRBACUnaryInterceptor(t *testing.T) {
	a, err := rbac.NewStatic(testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	interceptor := RBAC(a)
	next := func(ctx runtime.Ctx, req runtime.AnyRequest) (runtime.AnyResponse, error) {
		return nil, nil
	}
	wrapped := interceptor.WrapUnary(next)

	t.Run("admin allowed on any non-denied path", func(t *testing.T) {
		ctx := authedCtx("/acme.users.v1.UsersService/CreateUser", map[string]any{"role": "admin"})
		if _, err := wrapped(ctx, nil); err != nil {
			t.Fatalf("wrapped = %v, want nil", err)
		}
	})

	t.Run("anyone allowed on read paths", func(t *testing.T) {
		ctx := authedCtx("/acme.users.v1.UsersService/GetUser", map[string]any{"role": "user"})
		if _, err := wrapped(ctx, nil); err != nil {
			t.Fatalf("wrapped = %v, want nil", err)
		}
	})

	t.Run("deny rule beats allow rule", func(t *testing.T) {
		ctx := authedCtx("/acme.users.v1.UsersService/DeleteUser", map[string]any{"role": "admin"})
		_, err := wrapped(ctx, nil)
		if status.Code(err) != codes.CodePermissionDenied {
			t.Fatalf("Code = %v, want permission_denied", status.Code(err))
		}
	})

	t.Run("unauthenticated denied on non-public path", func(t *testing.T) {
		ctx := testCtx(nil, "/acme.users.v1.UsersService/CreateUser")
		_, err := wrapped(ctx, nil)
		if status.Code(err) != codes.CodePermissionDenied {
			t.Fatalf("Code = %v, want permission_denied", status.Code(err))
		}
	})
}

func TestRBACStreamInterceptor(t *testing.T) {
	a, err := rbac.NewStatic(testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	interceptor := RBACStream(a)

	t.Run("authorized stream passes through", func(t *testing.T) {
		ctx := authedCtx("/acme.users.v1.UsersService/GetUser", map[string]any{"role": "user"})
		stream := interceptor.WrapStream(&fakeStream{ctx: ctx})
		if err := stream.Send("msg"); err != nil {
			t.Fatalf("Send = %v, want nil", err)
		}
	})

	t.Run("denied stream returns error stream", func(t *testing.T) {
		ctx := authedCtx("/acme.users.v1.UsersService/DeleteUser", map[string]any{"role": "admin"})
		stream := interceptor.WrapStream(&fakeStream{ctx: ctx})
		if _, err := stream.Receive(); status.Code(err) != codes.CodePermissionDenied {
			t.Fatalf("Receive Code = %v, want permission_denied", status.Code(err))
		}
	})
}

func TestFileWatcherRBACStreamInterceptor(t *testing.T) {
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(policyFile, []byte(testPolicy), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := rbac.NewFileWatcher(policyFile, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	interceptor := RBACStream(a)
	ctx := authedCtx("/acme.users.v1.UsersService/GetUser", map[string]any{"role": "user"})
	stream := interceptor.WrapStream(&fakeStream{ctx: ctx})
	if err := stream.Send("msg"); err != nil {
		t.Fatalf("Send = %v, want nil", err)
	}
}
