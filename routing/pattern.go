// Package routing owns HTTP path matching and dispatch: the Mux, path
// templates, and the compressed radix tree for wildcard routes.
package routing

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Path template matching
// ---------------------------------------------------------------------------

// Pattern is a parsed HTTP path template in the style of grpc-gateway's
// google.api.http annotations. It supports:
//
//   - literal segments:        /v1/users
//   - single-segment params:   /v1/users/{id}
//   - resource name patterns:  /v1/{name=messages/*}
//   - multi-segment wildcards: /v1/{path=organizations/**}
//   - custom verb suffix:      /v1/example/{id}:custom
type Pattern struct {
	raw      string
	segments []segment
	verb     string
}

type segment struct {
	literal string
	param   *param
}

type param struct {
	name     string
	resource bool // true for {name=...} patterns
	expr     []exprSeg
}

type exprSeg struct {
	literal  string
	wildcard bool // single-segment *
	multi    bool // multi-segment **
}

// ParsePattern parses a path template into a Pattern.
func ParsePattern(template string) (*Pattern, error) {
	if template == "" {
		return nil, fmt.Errorf("parse pattern: empty template")
	}
	if !strings.HasPrefix(template, "/") {
		return nil, fmt.Errorf("parse pattern %q: must start with /", template)
	}

	p := &Pattern{raw: template}

	// Split off the custom verb suffix (:verb).
	rest := template
	if idx := strings.LastIndex(rest, ":"); idx > 0 {
		verb := rest[idx+1:]
		if verb != "" && !strings.Contains(verb, "/") && !strings.Contains(verb, "{") {
			p.verb = verb
			rest = rest[:idx]
		}
	}

	// Split into segments, respecting braces so resource name patterns
	// like {name=messages/*} stay intact.
	parts := splitTemplate(rest)
	for _, part := range parts {
		if part == "" {
			continue
		}
		seg, err := parseSegment(part)
		if err != nil {
			return nil, fmt.Errorf("parse pattern %q: %w", template, err)
		}
		p.segments = append(p.segments, seg)
	}
	if len(p.segments) == 0 {
		return nil, fmt.Errorf("parse pattern %q: no segments", template)
	}
	return p, nil
}

// splitTemplate splits a path template on "/" without splitting inside
// braces.
func splitTemplate(template string) []string {
	var parts []string
	var cur strings.Builder
	depth := 0
	for _, r := range template {
		switch r {
		case '{':
			depth++
			cur.WriteRune(r)
		case '}':
			depth--
			cur.WriteRune(r)
		case '/':
			if depth == 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			} else {
				cur.WriteRune(r)
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func parseSegment(part string) (segment, error) {
	if !strings.HasPrefix(part, "{") {
		return segment{literal: part}, nil
	}
	if !strings.HasSuffix(part, "}") {
		return segment{}, fmt.Errorf("invalid segment %q: missing closing brace", part)
	}
	inner := part[1 : len(part)-1]
	if inner == "" {
		return segment{}, fmt.Errorf("invalid segment %q: empty parameter", part)
	}

	// Resource name pattern: {name=messages/*}
	if eq := strings.Index(inner, "="); eq >= 0 {
		name := inner[:eq]
		expr := inner[eq+1:]
		if name == "" || expr == "" {
			return segment{}, fmt.Errorf("invalid segment %q: empty name or expression", part)
		}
		exprSegs, err := parseResourceExpr(expr)
		if err != nil {
			return segment{}, err
		}
		return segment{param: &param{name: name, resource: true, expr: exprSegs}}, nil
	}

	return segment{param: &param{name: inner}}, nil
}

// parseResourceExpr parses a resource name expression like
// "messages/*", "projects/*/topics/*", or "organizations/**".
func parseResourceExpr(expr string) ([]exprSeg, error) {
	var segs []exprSeg
	for _, part := range strings.Split(expr, "/") {
		switch part {
		case "**":
			segs = append(segs, exprSeg{multi: true})
		case "*":
			segs = append(segs, exprSeg{wildcard: true})
		case "":
			return nil, fmt.Errorf("invalid resource expression %q", expr)
		default:
			segs = append(segs, exprSeg{literal: part})
		}
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("invalid resource expression %q", expr)
	}
	return segs, nil
}

// Raw returns the original template string.
func (p *Pattern) Raw() string {
	return p.raw
}

// Verb returns the custom verb suffix (empty if none).
func (p *Pattern) Verb() string {
	return p.verb
}

// Match matches a URL path against the pattern. It returns the extracted
// path parameters and true on success. The params map is allocated lazily:
// patterns without parameters return a nil map.
func (p *Pattern) Match(path string) (map[string]string, bool) {
	var params map[string]string

	// Handle the custom verb suffix: the last segment must end with
	// ":verb", and the verb is stripped before matching.
	if p.verb != "" {
		lastStart := 0
		for i := 0; i < len(path); i++ {
			if path[i] == '/' {
				lastStart = i + 1
			}
		}
		if !strings.HasSuffix(path[lastStart:], ":"+p.verb) {
			return nil, false
		}
		path = path[:len(path)-len(p.verb)-1]
	}

	pi := 0 // path index
	for _, seg := range p.segments {
		for pi < len(path) && path[pi] == '/' {
			pi++
		}
		if seg.param != nil && seg.param.resource {
			// Resource name pattern: match the expression against the
			// remaining path and capture the full value.
			value, end, ok := matchResourceExpr(seg.param.expr, path, pi)
			if !ok {
				return nil, false
			}
			if params == nil {
				params = make(map[string]string)
			}
			params[seg.param.name] = value
			pi = end
			continue
		}
		if seg.param != nil {
			start := pi
			for pi < len(path) && path[pi] != '/' {
				pi++
			}
			if start == pi {
				return nil, false // empty segment
			}
			if params == nil {
				params = make(map[string]string)
			}
			params[seg.param.name] = path[start:pi]
			continue
		}
		if !strings.HasPrefix(path[pi:], seg.literal) {
			return nil, false
		}
		pi += len(seg.literal)
		// The literal must end at a segment boundary.
		if pi < len(path) && path[pi] != '/' {
			return nil, false
		}
	}

	// Skip trailing slashes; any remaining path means no match.
	for pi < len(path) && path[pi] == '/' {
		pi++
	}
	if pi != len(path) {
		return nil, false
	}
	return params, true
}

// matchResourceExpr matches a resource expression against the path starting
// at start. It returns the captured value (including the expression's
// literal prefix), the end index in path, and whether the match succeeded.
func matchResourceExpr(expr []exprSeg, path string, start int) (string, int, bool) {
	pi := start
	for _, es := range expr {
		for pi < len(path) && path[pi] == '/' {
			pi++
		}
		if es.multi {
			// Multi-segment wildcard consumes the rest; the captured
			// value includes the expression's literal prefix.
			return path[start:], len(path), true
		}
		if es.wildcard {
			for pi < len(path) && path[pi] != '/' {
				pi++
			}
			continue
		}
		if !strings.HasPrefix(path[pi:], es.literal) {
			return "", 0, false
		}
		pi += len(es.literal)
		if pi < len(path) && path[pi] != '/' {
			return "", 0, false
		}
	}
	return path[start:pi], pi, true
}

// String returns the raw template.
func (p *Pattern) String() string {
	return p.raw
}

// Specificity scores a pattern for route matching priority. Literal
// segments weigh more than params, and longer patterns weigh more than
// shorter ones. This makes /v1/users/me win over /v1/users/{id}.
func (p *Pattern) Specificity() int {
	score := 0
	for _, seg := range p.segments {
		if seg.param != nil {
			score += 1
		} else {
			score += 10
		}
	}
	return score
}
