package runtime

import "testing"

type substituteGetReq struct {
	ID string `json:"id"`
}

func TestSubstitutePathParams(t *testing.T) {
	got, err := SubstitutePathParams("/v1/users/{id}", &substituteGetReq{ID: "42"})
	if err != nil {
		t.Fatalf("substitute: %v", err)
	}
	if got != "/v1/users/42" {
		t.Errorf("got = %q, want /v1/users/42", got)
	}
}

func TestSubstitutePathParamsBound(t *testing.T) {
	tests := []struct {
		name     string
		template string
		req      any
		wantPath string
		wantBind []string
	}{
		{name: "single", template: "/v1/users/{id}", req: &substituteGetReq{ID: "42"}, wantPath: "/v1/users/42", wantBind: []string{"id"}},
		{name: "resource-name expression", template: "/v1/users/{id=users/*}", req: &substituteGetReq{ID: "42"}, wantPath: "/v1/users/42", wantBind: []string{"id"}},
		{name: "no params", template: "/v1/users", req: &substituteGetReq{ID: "42"}, wantPath: "/v1/users", wantBind: nil},
		{name: "nil req", template: "/v1/users/{id}", req: nil, wantPath: "/v1/users/{id}", wantBind: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, bound, err := SubstitutePathParamsBound(tt.template, tt.req)
			if err != nil {
				t.Fatalf("substitute: %v", err)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
			if len(bound) != len(tt.wantBind) {
				t.Fatalf("bound = %v, want %v", bound, tt.wantBind)
			}
			for i := range bound {
				if bound[i] != tt.wantBind[i] {
					t.Errorf("bound[%d] = %q, want %q", i, bound[i], tt.wantBind[i])
				}
			}
		})
	}
}
