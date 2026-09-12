package routing

import (
	"net/http"
	"testing"

	"github.com/aldok10/zara-rpc/kernel"
	"github.com/aldok10/zara-rpc/runtime"
)

// treeTestHandler builds a handler for a route tree test. The id is used
// as the RPC name so tests can identify which handler matched.
func treeTestHandler(id string) *kernel.Handler {
	op := kernel.NewOperation(
		http.MethodGet,
		"/unused",
		func(ctx runtime.Ctx, req *runtime.Request[testGetReq]) (*runtime.Response[testUser], error) {
			return runtime.NewResponse(&testUser{ID: id}), nil
		},
		kernel.WithRPC(id),
	)
	return kernel.NewHandler(op)
}

// mustTreePattern parses a pattern, failing the test on error.
func mustTreePattern(t *testing.T, template string) *Pattern {
	t.Helper()
	p, err := ParsePattern(template)
	if err != nil {
		t.Fatalf("ParsePattern(%q): %v", template, err)
	}
	return p
}

// insertRoute registers a pattern on the tree, failing the test on error.
func insertRoute(t *testing.T, tree *routeTree, template, id string) {
	t.Helper()
	tree.insert(mustTreePattern(t, template), treeTestHandler(id))
}

// lookupRoute walks the tree and returns the matched handler id and params.
func lookupRoute(t *testing.T, tree *routeTree, path string) (string, map[string]string) {
	t.Helper()
	h, params := tree.lookup(path)
	if h == nil {
		return "", params
	}
	return h.RPC(), params
}

func TestRouteTreeSimpleParam(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/users/{id}", "GetUser")

	got, params := lookupRoute(t, tree, "/v1/users/42")
	if got != "GetUser" {
		t.Fatalf("matched %q, want GetUser", got)
	}
	if params["id"] != "42" {
		t.Fatalf("params[id] = %q, want 42", params["id"])
	}

	// No match: different prefix.
	if got, _ := lookupRoute(t, tree, "/v2/users/42"); got != "" {
		t.Fatalf("matched %q for /v2/users/42, want no match", got)
	}
	// No match: empty segment.
	if got, _ := lookupRoute(t, tree, "/v1/users/"); got != "" {
		t.Fatalf("matched %q for /v1/users/, want no match", got)
	}
}

func TestRouteTreeMultipleParams(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/users/{id}/posts/{pid}", "GetPost")

	got, params := lookupRoute(t, tree, "/v1/users/42/posts/7")
	if got != "GetPost" {
		t.Fatalf("matched %q, want GetPost", got)
	}
	if params["id"] != "42" || params["pid"] != "7" {
		t.Fatalf("params = %v, want id=42 pid=7", params)
	}
}

func TestRouteTreeSpecificity(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/users/{id}", "GetUser")
	insertRoute(t, tree, "/v1/users/me", "GetMe")

	// Literal wins over param.
	got, _ := lookupRoute(t, tree, "/v1/users/me")
	if got != "GetMe" {
		t.Fatalf("matched %q for /v1/users/me, want GetMe", got)
	}
	// Param still matches other values.
	got, _ = lookupRoute(t, tree, "/v1/users/42")
	if got != "GetUser" {
		t.Fatalf("matched %q for /v1/users/42, want GetUser", got)
	}
}

func TestRouteTreeParamThenLiteral(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/{id}/posts", "GetPosts")
	insertRoute(t, tree, "/v1/{id}/comments", "GetComments")

	got, params := lookupRoute(t, tree, "/v1/42/posts")
	if got != "GetPosts" || params["id"] != "42" {
		t.Fatalf("got %q params %v, want GetPosts id=42", got, params)
	}
	got, _ = lookupRoute(t, tree, "/v1/42/comments")
	if got != "GetComments" {
		t.Fatalf("got %q, want GetComments", got)
	}
	// No match: unknown suffix.
	if got, _ := lookupRoute(t, tree, "/v1/42/likes"); got != "" {
		t.Fatalf("matched %q for /v1/42/likes, want no match", got)
	}
}

func TestRouteTreeResourcePattern(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/{name=messages/*}", "GetMessage")

	got, params := lookupRoute(t, tree, "/v1/messages/abc")
	if got != "GetMessage" {
		t.Fatalf("matched %q, want GetMessage", got)
	}
	if params["name"] != "messages/abc" {
		t.Fatalf("params[name] = %q, want messages/abc", params["name"])
	}

	// Wrong literal prefix inside the expression.
	if got, _ := lookupRoute(t, tree, "/v1/topics/abc"); got != "" {
		t.Fatalf("matched %q for /v1/topics/abc, want no match", got)
	}
	// The expression's wildcard may be empty (Pattern.Match semantics):
	// "messages" alone matches with value "messages".
	got, params = lookupRoute(t, tree, "/v1/messages")
	if got != "GetMessage" || params["name"] != "messages" {
		t.Fatalf("got %q params %v, want GetMessage name=messages (empty wildcard)", got, params)
	}
}

func TestRouteTreeMultiSegmentWildcard(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/{path=organizations/**}", "GetOrgPath")

	got, params := lookupRoute(t, tree, "/v1/organizations/acme/team")
	if got != "GetOrgPath" {
		t.Fatalf("matched %q, want GetOrgPath", got)
	}
	if params["path"] != "organizations/acme/team" {
		t.Fatalf("params[path] = %q, want organizations/acme/team", params["path"])
	}

	// Wrong literal prefix.
	if got, _ := lookupRoute(t, tree, "/v1/companies/acme"); got != "" {
		t.Fatalf("matched %q for /v1/companies/acme, want no match", got)
	}
}

func TestRouteTreeCustomVerb(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/example/{id}:custom", "CustomAction")

	got, params := lookupRoute(t, tree, "/v1/example/123:custom")
	if got != "CustomAction" {
		t.Fatalf("matched %q, want CustomAction", got)
	}
	if params["id"] != "123" {
		t.Fatalf("params[id] = %q, want 123 (verb stripped)", params["id"])
	}

	// Missing the verb.
	if got, _ := lookupRoute(t, tree, "/v1/example/123"); got != "" {
		t.Fatalf("matched %q for /v1/example/123, want no match", got)
	}
	// Wrong verb.
	if got, _ := lookupRoute(t, tree, "/v1/example/123:other"); got != "" {
		t.Fatalf("matched %q for /v1/example/123:other, want no match", got)
	}
}

func TestRouteTreeVerbAndPlainShareParam(t *testing.T) {
	tree := newRouteTree()
	// Register the verb-ful handler first: first registration wins when
	// both terminals match, so the verb must be tried before the plain
	// terminal for "/v1/example/123:custom" to reach it.
	insertRoute(t, tree, "/v1/example/{id}:custom", "CustomAction")
	insertRoute(t, tree, "/v1/example/{id}", "GetExample")

	got, params := lookupRoute(t, tree, "/v1/example/123:custom")
	if got != "CustomAction" || params["id"] != "123" {
		t.Fatalf("got %q params %v, want CustomAction id=123", got, params)
	}
	got, params = lookupRoute(t, tree, "/v1/example/123")
	if got != "GetExample" || params["id"] != "123" {
		t.Fatalf("got %q params %v, want GetExample id=123", got, params)
	}
}

func TestRouteTreeTrailingSlash(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/users/{id}", "GetUser")

	got, params := lookupRoute(t, tree, "/v1/users/42/")
	if got != "GetUser" || params["id"] != "42" {
		t.Fatalf("got %q params %v, want GetUser id=42 (trailing slash)", got, params)
	}
}

func TestRouteTreeFirstRegistrationWins(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/users/{id}", "First")
	insertRoute(t, tree, "/v1/users/{id}", "Second")

	got, _ := lookupRoute(t, tree, "/v1/users/42")
	if got != "First" {
		t.Fatalf("matched %q, want First (first registration wins)", got)
	}
}

func TestRouteTreePrefixSplit(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/users/{id}", "GetUser")
	insertRoute(t, tree, "/v1/user/{name}", "GetByName")

	got, params := lookupRoute(t, tree, "/v1/users/42")
	if got != "GetUser" || params["id"] != "42" {
		t.Fatalf("got %q params %v, want GetUser id=42", got, params)
	}
	got, params = lookupRoute(t, tree, "/v1/user/aldo")
	if got != "GetByName" || params["name"] != "aldo" {
		t.Fatalf("got %q params %v, want GetByName name=aldo", got, params)
	}
}

func TestRouteTreeSharedPrefixDeep(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/users/{id}", "GetUser")
	insertRoute(t, tree, "/v1/users/me", "GetMe")
	insertRoute(t, tree, "/v1/users/me/posts", "GetMyPosts")

	got, _ := lookupRoute(t, tree, "/v1/users/me")
	if got != "GetMe" {
		t.Fatalf("got %q, want GetMe", got)
	}
	got, _ = lookupRoute(t, tree, "/v1/users/me/posts")
	if got != "GetMyPosts" {
		t.Fatalf("got %q, want GetMyPosts", got)
	}
	got, _ = lookupRoute(t, tree, "/v1/users/42")
	if got != "GetUser" {
		t.Fatalf("got %q, want GetUser", got)
	}
}

func TestRouteTreeResourceWithChildren(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/{name=messages/*}/comments", "GetComments")

	got, params := lookupRoute(t, tree, "/v1/messages/abc/comments")
	if got != "GetComments" {
		t.Fatalf("matched %q, want GetComments", got)
	}
	if params["name"] != "messages/abc" {
		t.Fatalf("params[name] = %q, want messages/abc", params["name"])
	}
}

func TestRouteTreeResourceVerb(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/{name=messages/*}:custom", "CustomMessage")

	got, params := lookupRoute(t, tree, "/v1/messages/abc:custom")
	if got != "CustomMessage" {
		t.Fatalf("matched %q, want CustomMessage", got)
	}
	if params["name"] != "messages/abc" {
		t.Fatalf("params[name] = %q, want messages/abc (verb stripped)", params["name"])
	}
}

func TestRouteTreeNoMatch(t *testing.T) {
	tree := newRouteTree()
	insertRoute(t, tree, "/v1/users/{id}", "GetUser")

	for _, path := range []string{
		"/",
		"/v1",
		"/v1/users",
		"/v1/users/42/extra",
		"/v2/users/42",
		"/v1/groups/42",
	} {
		if got, _ := lookupRoute(t, tree, path); got != "" {
			t.Fatalf("matched %q for %q, want no match", got, path)
		}
	}
}

func TestRouteTreeEmptyTree(t *testing.T) {
	tree := newRouteTree()
	if got, _ := lookupRoute(t, tree, "/v1/users/42"); got != "" {
		t.Fatalf("matched %q on empty tree, want no match", got)
	}
}
