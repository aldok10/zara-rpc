// Path template parsing and path-parameter extraction.
package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// firstHTTPRule returns the first HTTP binding for a method, panicking if
// the method has none. Used by client generation.
func firstHTTPRule(method *protogen.Method) httpRule {
	rules, err := extractHTTPRules(method)
	if err != nil {
		panic(err)
	}
	if len(rules) == 0 {
		panic(fmt.Sprintf("method %s has no HTTP binding", method.GoName))
	}
	return rules[0]
}

// httpMethodConst maps an HTTP method name to the net/http constant.
func httpMethodConst(g *protogen.GeneratedFile, method string) string {
	switch method {
	case "GET":
		return q(g, httpImport, "MethodGet")
	case "POST":
		return q(g, httpImport, "MethodPost")
	case "PUT":
		return q(g, httpImport, "MethodPut")
	case "DELETE":
		return q(g, httpImport, "MethodDelete")
	case "PATCH":
		return q(g, httpImport, "MethodPatch")
	case "HEAD":
		return q(g, httpImport, "MethodHead")
	case "OPTIONS":
		return q(g, httpImport, "MethodOptions")
	default:
		return strconvQuote(method)
	}
}

// pathParam is a parsed path parameter from a template.
type pathParam struct {
	name string
	path string // dot-separated field path
}

// pathParamsFor extracts path parameters from a path template.
func pathParamsFor(template string) ([]pathParam, error) {
	var params []pathParam
	// Find all {name} or {name=expr} segments.
	rest := template
	for {
		start := strings.Index(rest, "{")
		if start < 0 {
			break
		}
		end := strings.Index(rest[start:], "}")
		if end < 0 {
			return nil, fmt.Errorf("unclosed brace in %q", template)
		}
		inner := rest[start+1 : start+end]
		name := inner
		if eq := strings.Index(inner, "="); eq >= 0 {
			name = inner[:eq]
		}
		params = append(params, pathParam{name: name, path: name})
		rest = rest[start+end+1:]
	}
	return params, nil
}

// fieldPath is a resolved dot-separated field path into a message.
type fieldPath struct {
	fields []*protogen.Field
}

// resolveFieldPath resolves a dot-separated field path like "status.note"
// into concrete protogen fields.
func resolveFieldPath(msg *protogen.Message, path string) (*fieldPath, error) {
	parts := strings.Split(path, ".")
	fp := &fieldPath{}
	cur := msg
	for _, part := range parts {
		var found *protogen.Field
		for _, f := range cur.Fields {
			if string(f.Desc.Name()) == part {
				found = f
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("field %q not found in message %s", part, cur.Desc.Name())
		}
		fp.fields = append(fp.fields, found)
		if found.Message != nil {
			cur = found.Message
		}
	}
	return fp, nil
}

// goExpr returns the Go expression for the field path rooted at prefix.
func (fp *fieldPath) goExpr(prefix string) string {
	expr := prefix
	for _, f := range fp.fields {
		expr += "." + f.GoName
	}
	return expr
}

// prepStatements returns statements that allocate nil message pointers
// along the field path.
func (fp *fieldPath) prepStatements(g *protogen.GeneratedFile, prefix string) []string {
	var stmts []string
	expr := prefix
	for _, f := range fp.fields[:len(fp.fields)-1] {
		expr += "." + f.GoName
		if f.Message != nil {
			stmts = append(stmts, fmt.Sprintf("if %s == nil {", expr))
			stmts = append(stmts, fmt.Sprintf("%s = &%s{}", expr, g.QualifiedGoIdent(f.Message.GoIdent)))
			stmts = append(stmts, "}")
		}
	}
	return stmts
}

// genPathParamExtraction emits code that extracts one path parameter and
// assigns it to the message field, with type-safe conversion. String fields
// are assigned directly (PathString is the identity function — calling it
// would add a function call and error plumbing for nothing); typed fields
// call the runtime converter and cast to the field type.
func genPathParamExtraction(g *protogen.GeneratedFile, msg *protogen.Message, pp pathParam, prefix string) {
	fp, err := resolveFieldPath(msg, pp.path)
	if err != nil {
		panic(err)
	}
	field := fp.fields[len(fp.fields)-1]
	expr := fp.goExpr(prefix)

	g.P("if v, ok := params[", strconvQuote(pp.name), "]; ok {")
	for _, stmt := range fp.prepStatements(g, prefix) {
		g.P(stmt)
	}

	kind := field.Desc.Kind()
	repeated := field.Desc.IsList()

	if kind == protoreflect.StringKind && !repeated {
		g.P(expr, " = v")
		g.P("}")
		return
	}

	converter, tmpType, err := converterFor(g, field)
	if err != nil {
		panic(err)
	}
	g.P("s, err := ", converter, "(v)")
	g.P("if err != nil {")
	g.P("return nil, ", sym(g, "NewErrorf"), "(", sym(g, "CodeInvalidArgument"), ", \"type mismatch, parameter: ", pp.name, ", error: %v\", err)")
	g.P("}")
	// Enum converters return int32; cast to the enum type. Repeated
	// converters return the slice type directly.
	if kind == protoreflect.EnumKind && !repeated {
		g.P(expr, " = ", tmpType, "(s)")
	} else {
		g.P(expr, " = s")
	}
	g.P("}")
}

// converterFor returns the zararpc converter function and the Go type of
// its result for a proto field.
func converterFor(g *protogen.GeneratedFile, field *protogen.Field) (converter, tmpType string, err error) {
	kind := field.Desc.Kind()
	repeated := field.Desc.IsList()

	// Enum fields: convert via int32 then cast to the enum type.
	if kind == protoreflect.EnumKind && !repeated {
		return sym(g, "PathInt32"), g.QualifiedGoIdent(field.GoIdent), nil
	}

	switch kind {
	case protoreflect.StringKind:
		if repeated {
			return sym(g, "PathStringSlice"), "[]string", nil
		}
		return sym(g, "PathString"), "string", nil
	case protoreflect.BoolKind:
		return sym(g, "PathBool"), "bool", nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return sym(g, "PathInt32"), "int32", nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		if repeated {
			return sym(g, "PathInt64Slice"), "[]int64", nil
		}
		return sym(g, "PathInt64"), "int64", nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return sym(g, "PathUint32"), "uint32", nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return sym(g, "PathUint64"), "uint64", nil
	case protoreflect.FloatKind:
		return sym(g, "PathFloat32"), "float32", nil
	case protoreflect.DoubleKind:
		return sym(g, "PathFloat64"), "float64", nil
	case protoreflect.BytesKind:
		return sym(g, "PathBytes"), "[]byte", nil
	default:
		return "", "", fmt.Errorf("unsupported path param type %s for field %s", kind, field.Desc.Name())
	}
}
