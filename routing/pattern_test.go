package routing

import (
	"reflect"
	"testing"
)

func TestParsePattern(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{name: "literal", template: "/v1/users"},
		{name: "single param", template: "/v1/users/{id}"},
		{name: "multiple params", template: "/v1/users/{id}/posts/{post_id}"},
		{name: "resource name", template: "/v1/{name=messages/*}"},
		{name: "resource multi", template: "/v1/{name=projects/*/topics/*}"},
		{name: "wildcard", template: "/v1/{path=organizations/**}"},
		{name: "verb", template: "/v1/example/{id}:custom"},
		{name: "nested field", template: "/v1/example/{status.note}"},
		{name: "empty", template: "", wantErr: true},
		{name: "no leading slash", template: "v1/users", wantErr: true},
		{name: "unclosed brace", template: "/v1/users/{id", wantErr: true},
		{name: "empty param", template: "/v1/users/{}", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParsePattern(tt.template)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePattern(%q) = nil error, want error", tt.template)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePattern(%q) = %v, want nil", tt.template, err)
			}
			if p.Raw() != tt.template {
				t.Errorf("Raw() = %q, want %q", p.Raw(), tt.template)
			}
		})
	}
}

func TestPatternMatch(t *testing.T) {
	tests := []struct {
		name     string
		template string
		path     string
		want     map[string]string
		wantOK   bool
	}{
		{
			name:     "literal exact",
			template: "/v1/users",
			path:     "/v1/users",
			want:     nil, // no params captured: Match returns a nil map
			wantOK:   true,
		},
		{
			name:     "literal mismatch",
			template: "/v1/users",
			path:     "/v1/posts",
			wantOK:   false,
		},
		{
			name:     "single param",
			template: "/v1/users/{id}",
			path:     "/v1/users/42",
			want:     map[string]string{"id": "42"},
			wantOK:   true,
		},
		{
			name:     "param too few segments",
			template: "/v1/users/{id}",
			path:     "/v1/users",
			wantOK:   false,
		},
		{
			name:     "param too many segments",
			template: "/v1/users/{id}",
			path:     "/v1/users/42/extra",
			wantOK:   false,
		},
		{
			name:     "multiple params",
			template: "/v1/users/{id}/posts/{post_id}",
			path:     "/v1/users/42/posts/7",
			want:     map[string]string{"id": "42", "post_id": "7"},
			wantOK:   true,
		},
		{
			name:     "resource name",
			template: "/v1/{name=messages/*}",
			path:     "/v1/messages/123456",
			want:     map[string]string{"name": "messages/123456"},
			wantOK:   true,
		},
		{
			name:     "resource name wrong prefix",
			template: "/v1/{name=messages/*}",
			path:     "/v1/topics/123456",
			wantOK:   false,
		},
		{
			name:     "resource multi",
			template: "/v1/{name=projects/*/topics/*}",
			path:     "/v1/projects/my-proj/topics/my-topic",
			want:     map[string]string{"name": "projects/my-proj/topics/my-topic"},
			wantOK:   true,
		},
		{
			name:     "wildcard",
			template: "/v1/{path=organizations/**}",
			path:     "/v1/organizations/acme/teams/eng",
			want:     map[string]string{"path": "organizations/acme/teams/eng"},
			wantOK:   true,
		},
		{
			name:     "wildcard single segment",
			template: "/v1/{path=organizations/**}",
			path:     "/v1/organizations/acme",
			want:     map[string]string{"path": "organizations/acme"},
			wantOK:   true,
		},
		{
			name:     "verb",
			template: "/v1/example/{id}:custom",
			path:     "/v1/example/42:custom",
			want:     map[string]string{"id": "42"},
			wantOK:   true,
		},
		{
			name:     "verb mismatch",
			template: "/v1/example/{id}:custom",
			path:     "/v1/example/42",
			wantOK:   false,
		},
		{
			name:     "nested field",
			template: "/v1/example/{status.note}",
			path:     "/v1/example/hello",
			want:     map[string]string{"status.note": "hello"},
			wantOK:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParsePattern(tt.template)
			if err != nil {
				t.Fatalf("ParsePattern: %v", err)
			}
			got, ok := p.Match(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("Match(%q) ok = %v, want %v", tt.path, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestPatternVerb(t *testing.T) {
	p, err := ParsePattern("/v1/example/{id}:custom")
	if err != nil {
		t.Fatal(err)
	}
	if p.Verb() != "custom" {
		t.Errorf("Verb() = %q, want %q", p.Verb(), "custom")
	}
}
