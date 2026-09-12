package routing

import (
	"strings"

	"github.com/aldok10/zara-rpc/kernel"
)

// nodeKind distinguishes the three segment kinds in a route tree.
type nodeKind uint8

const (
	nodeLiteral nodeKind = iota
	nodeParam
	nodeResource
)

// routeTree is a compressed radix tree for wildcard route matching. One
// tree per HTTP method. Lookup is O(k) in path length, independent of the
// number of registered routes: literal prefixes are shared, and the walk
// collects path parameters in a single pass (Pattern.Match is not re-run).
//
// The tree preserves the mux's routing semantics structurally:
//   - literal children are tried before param/resource children, so the
//     most specific match wins (e.g. /v1/users/me over /v1/users/{id});
//   - duplicate patterns keep first-registration-wins;
//   - custom verbs (:custom) and resource name patterns ({name=messages/*},
//     {path=organizations/**}) are matched during the walk.
type routeTree struct {
	root *routeNode
}

// routeNode is one node in the route tree. Literal nodes match a static
// prefix; param nodes capture one path segment; resource nodes match a
// resource name expression.
type routeNode struct {
	kind      nodeKind
	prefix    string    // nodeLiteral: static prefix (slashes included)
	name      string    // nodeParam/nodeResource: parameter name
	expr      []exprSeg // nodeResource: resource expression
	terminals []routeTerminal
	children  []*routeNode
}

// routeTerminal is a route ending at a node: the handler and its custom
// verb suffix ("" when the pattern has no verb).
type routeTerminal struct {
	verb    string
	handler *kernel.Handler
}

func newRouteTree() *routeTree {
	return &routeTree{root: &routeNode{}}
}

// insert registers a pattern's handler. First registration wins for
// duplicate patterns (same segments and verb).
func (t *routeTree) insert(pattern *Pattern, h *kernel.Handler) {
	node := t.root
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			node = node.addLiteralChild(lit.String())
			lit.Reset()
		}
	}
	for _, seg := range pattern.segments {
		if seg.param != nil {
			flush()
			if seg.param.resource {
				node = node.addResourceChild(seg.param.name, seg.param.expr)
			} else {
				node = node.addParamChild(seg.param.name)
			}
		} else {
			lit.WriteByte('/')
			lit.WriteString(seg.literal)
		}
	}
	flush()
	node.addTerminal(pattern.verb, h)
}

// lookup walks the tree for the most specific handler matching path. It
// returns the handler and the extracted path parameters (nil when the
// pattern has no parameters or the path does not match).
func (t *routeTree) lookup(path string) (*kernel.Handler, map[string]string) {
	return t.root.lookup(path, nil)
}

func (n *routeNode) lookup(path string, params map[string]string) (*kernel.Handler, map[string]string) {
	switch n.kind {
	case nodeLiteral:
		if !strings.HasPrefix(path, n.prefix) {
			return nil, params
		}
		path = path[len(n.prefix):]
	case nodeParam:
		// Skip the separator before this segment. Literal prefixes do
		// not include the trailing slash, so the path here starts with
		// "/" when a literal preceded this param.
		for len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}
		end := strings.IndexByte(path, '/')
		seg := path
		rest := ""
		if end >= 0 {
			seg = path[:end]
			rest = path[end:]
		}
		if seg == "" {
			return nil, params
		}
		if params == nil {
			params = make(map[string]string)
		}
		// Terminal check first: the custom verb, when present, is a suffix
		// of the captured segment.
		if h, ok := n.matchParamTerminal(seg, rest, params); ok {
			return h, params
		}
		params[n.name] = seg
		for _, child := range n.children {
			if h, p := child.lookup(rest, params); h != nil {
				return h, p
			}
		}
		delete(params, n.name)
		return nil, params
	case nodeResource:
		// Skip the separator before this segment, like nodeParam. The
		// captured value must not include the leading slash.
		for len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}
		value, end, ok := matchResourceExpr(n.expr, path, 0)
		if !ok {
			return nil, params
		}
		rest := path[end:]
		if params == nil {
			params = make(map[string]string)
		}
		if h, ok := n.matchResourceTerminal(value, rest, params); ok {
			return h, params
		}
		params[n.name] = value
		for _, child := range n.children {
			if h, p := child.lookup(rest, params); h != nil {
				return h, p
			}
		}
		delete(params, n.name)
		return nil, params
	}

	// Literal node: terminal check, then children.
	if h, ok := n.matchTerminal(path, params); ok {
		return h, params
	}
	for _, child := range n.children {
		if h, p := child.lookup(path, params); h != nil {
			return h, p
		}
	}
	return nil, params
}

// matchTerminal checks literal terminals: the remaining path must be empty
// (trailing slashes ignored) or end with the custom verb.
func (n *routeNode) matchTerminal(path string, params map[string]string) (*kernel.Handler, bool) {
	path = stripTrailingSlashes(path)
	for _, t := range n.terminals {
		if t.verb == "" {
			if path == "" {
				return t.handler, true
			}
		} else if strings.HasSuffix(path, ":"+t.verb) && path[:len(path)-len(t.verb)-1] == "" {
			return t.handler, true
		}
	}
	return nil, false
}

// matchParamTerminal checks param terminals. The custom verb, when present,
// is a suffix of the captured segment (e.g. /v1/example/{id}:custom).
func (n *routeNode) matchParamTerminal(seg, rest string, params map[string]string) (*kernel.Handler, bool) {
	rest = stripTrailingSlashes(rest)
	for _, t := range n.terminals {
		if t.verb == "" {
			if rest == "" {
				params[n.name] = seg
				return t.handler, true
			}
		} else if rest == "" && strings.HasSuffix(seg, ":"+t.verb) {
			params[n.name] = seg[:len(seg)-len(t.verb)-1]
			return t.handler, true
		}
	}
	return nil, false
}

// matchResourceTerminal checks resource terminals. The custom verb, when
// present, is a suffix of the captured value.
func (n *routeNode) matchResourceTerminal(value, rest string, params map[string]string) (*kernel.Handler, bool) {
	rest = stripTrailingSlashes(rest)
	for _, t := range n.terminals {
		if t.verb == "" {
			if rest == "" {
				params[n.name] = value
				return t.handler, true
			}
		} else if rest == "" && strings.HasSuffix(value, ":"+t.verb) {
			params[n.name] = value[:len(value)-len(t.verb)-1]
			return t.handler, true
		}
	}
	return nil, false
}

// addLiteralChild returns the child node for a literal prefix, splitting
// existing nodes when the new prefix shares only part of a prefix. New
// literal children are inserted before the first non-literal child so the
// walk tries literals first (most specific wins).
func (n *routeNode) addLiteralChild(prefix string) *routeNode {
	for _, child := range n.children {
		if child.kind != nodeLiteral {
			continue
		}
		common := commonPrefixLen(child.prefix, prefix)
		if common == 0 {
			continue
		}
		if common == len(child.prefix) && common == len(prefix) {
			return child
		}
		if common == len(child.prefix) {
			return child.addLiteralChild(prefix[common:])
		}
		// Split the child: the common prefix becomes the child, the
		// remainder becomes a new literal child of it.
		split := &routeNode{kind: nodeLiteral, prefix: child.prefix[common:]}
		split.terminals = child.terminals
		split.children = child.children
		child.prefix = child.prefix[:common]
		child.terminals = nil
		child.children = []*routeNode{split}
		if common == len(prefix) {
			return child
		}
		return child.addLiteralChild(prefix[common:])
	}
	// No shared prefix: insert a new literal child before the first
	// non-literal child.
	child := &routeNode{kind: nodeLiteral, prefix: prefix}
	idx := len(n.children)
	for i, c := range n.children {
		if c.kind != nodeLiteral {
			idx = i
			break
		}
	}
	n.children = append(n.children, nil)
	copy(n.children[idx+1:], n.children[idx:])
	n.children[idx] = child
	return child
}

// addParamChild returns the param child for a parameter name, creating it
// if absent. Param children are appended after literal children.
func (n *routeNode) addParamChild(name string) *routeNode {
	for _, child := range n.children {
		if child.kind == nodeParam && child.name == name {
			return child
		}
	}
	child := &routeNode{kind: nodeParam, name: name}
	n.children = append(n.children, child)
	return child
}

// addResourceChild returns the resource child for a resource expression,
// creating it if absent. Resource children are appended after param
// children.
func (n *routeNode) addResourceChild(name string, expr []exprSeg) *routeNode {
	for _, child := range n.children {
		if child.kind == nodeResource && child.name == name && exprEqual(child.expr, expr) {
			return child
		}
	}
	child := &routeNode{kind: nodeResource, name: name, expr: expr}
	n.children = append(n.children, child)
	return child
}

// addTerminal registers a route ending at this node. First registration
// wins for duplicate (verb, handler) pairs.
func (n *routeNode) addTerminal(verb string, h *kernel.Handler) {
	for _, t := range n.terminals {
		if t.verb == verb {
			return
		}
	}
	n.terminals = append(n.terminals, routeTerminal{verb: verb, handler: h})
}

// stripTrailingSlashes removes trailing '/' characters. Pattern.Match
// treats trailing slashes as insignificant, so the tree does too.
func stripTrailingSlashes(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// commonPrefixLen returns the length of the longest common prefix of a
// and b.
func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// exprEqual reports whether two resource expressions are identical.
func exprEqual(a, b []exprSeg) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
