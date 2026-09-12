// Handler and request-builder emission.
package main

import (
	"sort"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

// streamMode returns the streaming mode of a method.
func streamMode(method *protogen.Method) string {
	switch {
	case method.Desc.IsStreamingServer() && method.Desc.IsStreamingClient():
		return "bidi"
	case method.Desc.IsStreamingServer():
		return "server"
	case method.Desc.IsStreamingClient():
		return "client"
	default:
		return "unary"
	}
}

// generateHandlerMethod emits one method signature in either the handler
// interface (unimplemented=false) or as an Unimplemented method
// (unimplemented=true).
func generateHandlerMethod(g *protogen.GeneratedFile, svcName string, method *protogen.Method, ctxType string, unimplemented bool) {
	name := method.GoName
	inType := "*" + g.QualifiedGoIdent(method.Input.GoIdent)
	outType := "*" + g.QualifiedGoIdent(method.Output.GoIdent)
	mode := streamMode(method)

	switch mode {
	case "server":
		streamType := sym(g, "ServerStream") + "[*" + g.QualifiedGoIdent(method.Output.GoIdent) + "]"
		if unimplemented {
			g.P("func (Unimplemented", svcName, "Handler) ", name, "(", ctxType, ", ", inType, ", ", streamType, ") error {")
			g.P("return ", sym(g, "NewErrorf"), "(", sym(g, "CodeUnimplemented"), ", \"method ", name, " not implemented\")")
			g.P("}")
			g.P()
		} else {
			g.P(name, "(", ctxType, ", ", inType, ", ", streamType, ") error")
		}

	case "client":
		streamType := sym(g, "ClientStream") + "[*" + g.QualifiedGoIdent(method.Input.GoIdent) + "]"
		if unimplemented {
			g.P("func (Unimplemented", svcName, "Handler) ", name, "(", ctxType, ", ", streamType, ") (", outType, ", error) {")
			g.P("return nil, ", sym(g, "NewErrorf"), "(", sym(g, "CodeUnimplemented"), ", \"method ", name, " not implemented\")")
			g.P("}")
			g.P()
		} else {
			g.P(name, "(", ctxType, ", ", streamType, ") (", outType, ", error)")
		}

	case "bidi":
		streamType := sym(g, "BidiStream") + "[*" + g.QualifiedGoIdent(method.Input.GoIdent) + ", *" + g.QualifiedGoIdent(method.Output.GoIdent) + "]"
		if unimplemented {
			g.P("func (Unimplemented", svcName, "Handler) ", name, "(", ctxType, ", ", streamType, ") error {")
			g.P("return ", sym(g, "NewErrorf"), "(", sym(g, "CodeUnimplemented"), ", \"method ", name, " not implemented\")")
			g.P("}")
			g.P()
		} else {
			g.P(name, "(", ctxType, ", ", streamType, ") error")
		}

	default: // unary
		if unimplemented {
			g.P("func (Unimplemented", svcName, "Handler) ", name, "(", ctxType, ", ", inType, ") (", outType, ", error) {")
			g.P("return nil, ", sym(g, "NewErrorf"), "(", sym(g, "CodeUnimplemented"), ", \"method ", name, " not implemented\")")
			g.P("}")
			g.P()
		} else {
			g.P(name, "(", ctxType, ", ", inType, ") (", outType, ", error)")
		}
	}
}

// generateRequestBuilder emits a function that builds a typed request from
// an HTTP request, extracting path params type-safely. ctx is the
// runtime.Ctx built by the mux; the builder reads its metadata directly
// instead of r.Context() so the hot path never re-wraps the request with
// r.WithContext.
//
// The builder is the zero-alloc analogue of the reflection-based
// PopulateMessage: the generator knows the message fields and their types,
// so it emits direct field assignments instead of reflect lookups. The
// message itself still escapes to the handler (the framework contract), but
// no intermediate reflection state, variadic bound slices, or interface
// boxing is allocated per request.
func generateRequestBuilder(g *protogen.GeneratedFile, svc *protogen.Service, method *protogen.Method, rule httpRule, idx int) {
	fnName := requestBuilderName(svc.GoName, method.GoName, idx)
	httpReq := q(g, httpImport, "Request")
	ctxType := q(g, runtimeImport, "Ctx")
	specType := sym(g, "Spec")
	codecType := sym(g, "Codec")
	anyReq := sym(g, "AnyRequest")

	g.P("func ", fnName, "(ctx ", ctxType, ", r *", httpReq, ", params map[string]string, spec ", specType, ", codec ", codecType, ") (", anyReq, ", error) {")
	g.P("msg := &", g.QualifiedGoIdent(method.Input.GoIdent), "{}")
	g.P("meta := ctx.Meta()")
	g.P()

	// Body first, then path params: protojson.Unmarshal resets fields
	// absent from the JSON, so unmarshaling after path-param assignment
	// would drop the path params on routes that carry both a body and path
	// params. Path params are applied last so they win over the body
	// (grpc-gateway semantics).
	switch rule.body {
	case "*":
		g.P("// Body: entire request message (buffered by the mux).")
		g.P("if err := codec.Unmarshal(meta.Body, msg); err != nil {")
		g.P("return nil, ", sym(g, "NewErrorf"), "(", sym(g, "CodeInvalidArgument"), ", \"decode request body: %v\", err)")
		g.P("}")
		g.P()
	case "":
		// No body.
	default:
		g.P("// Body: field ", strconvQuote(rule.body), ".")
		fp, err := resolveFieldPath(method.Input, rule.body)
		if err != nil {
			panic(err)
		}
		for _, stmt := range fp.prepStatements(g, "msg") {
			g.P(stmt)
		}
		expr := fp.goExpr("msg")
		g.P("if err := codec.Unmarshal(meta.Body, &", expr, "); err != nil {")
		g.P("return nil, ", sym(g, "NewErrorf"), "(", sym(g, "CodeInvalidArgument"), ", \"decode request body: %v\", err)")
		g.P("}")
		g.P()
	}

	// Path params.
	pathParams, err := pathParamsFor(rule.path)
	if err != nil {
		panic(err)
	}
	if len(pathParams) > 0 {
		g.P("// Path parameters.")
		for _, pp := range pathParams {
			genPathParamExtraction(g, method.Input, pp, "msg")
		}
		g.P()
	}

	// Query params. Per google.api.http rules, every request field not bound
	// by the path template or the body automatically becomes a query
	// parameter (nested dot-paths included, e.g. sub.subfield=foo). When
	// body is "*" there are no query parameters at all. runtime.PopulateQuery
	// binds by reflection and ignores unknown query parameters, matching
	// grpc-gateway.
	if rule.body != "*" {
		bound := map[string]bool{}
		for _, pp := range pathParams {
			bound[pp.name] = true
		}
		if rule.body != "" {
			bound[rule.body] = true
		}
		g.P("// Query parameters (fields not bound by path or body).")
		g.P("if err := ", sym(g, "PopulateQuery"), "(msg, meta.Query", boundArgs(bound), "); err != nil {")
		g.P("return nil, err")
		g.P("}")
		g.P()
	}

	g.P("return ", sym(g, "NewRequestWithMeta"), "(msg, meta.Header, spec, ", sym(g, "Peer"), "{Addr: r.RemoteAddr, Protocol: r.Proto}), nil")
	g.P("}")
	g.P()
}

// boundArgs renders the bound-name variadic for PopulateQuery as a
// comma-separated string list ("" when empty). The map iteration order is
// nondeterministic, so names are sorted for stable output.
func boundArgs(bound map[string]bool) string {
	if len(bound) == 0 {
		return ""
	}
	names := make([]string, 0, len(bound))
	for name := range bound {
		names = append(names, name)
	}
	sort.Strings(names)
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, strconvQuote(name))
	}
	return ", " + strings.Join(quoted, ", ")
}
