// Operation constructor emission.
package main

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
)

// operationFnName returns the name of the generated operation constructor
// for a method binding. The first binding has no suffix; additional
// bindings (additional_bindings) get an index suffix.
func operationFnName(svcName, methodName string, idx int) string {
	if idx == 0 {
		return "Register_" + methodName
	}
	return fmt.Sprintf("Register_%s_%d", methodName, idx)
}

// rpcNameConstName returns the name of the generated RPC name constant for
// a method.
func rpcNameConstName(svcName, methodName string) string {
	return svcName + "_" + methodName + "_Method"
}

// pathConstName returns the name of the generated path constant for a
// method binding.
func pathConstName(svcName, methodName string, idx int) string {
	if idx == 0 {
		return svcName + "_" + methodName + "_Path"
	}
	return fmt.Sprintf("%s_%s_Path_%d", svcName, methodName, idx)
}

// requestBuilderName returns the name of the generated request builder for
// a method binding.
func requestBuilderName(svcName, methodName string, idx int) string {
	return fmt.Sprintf("request_%s_%s_%d", svcName, methodName, idx)
}

// generateOperationFn emits a named operation constructor for one HTTP
// binding, e.g.:
//
//	func Register_GetUser(svc UsersServiceHandler) *zara_rpc.Operation {
//		return zara_rpc.NewOperation(...)
//	}
func generateOperationFn(g *protogen.GeneratedFile, svc *protogen.Service, method *protogen.Method, rule httpRule, idx int) {
	fnName := operationFnName(svc.GoName, method.GoName, idx)
	ctxType := q(g, runtimeImport, "Ctx")
	mode := streamMode(method)

	switch mode {
	case "server":
		generateServerStreamOperationFn(g, svc, method, rule, idx, fnName, ctxType)
	case "client":
		generateClientStreamOperationFn(g, svc, method, rule, idx, fnName, ctxType)
	case "bidi":
		generateBidiStreamOperationFn(g, svc, method, rule, idx, fnName, ctxType)
	default:
		generateUnaryOperationFn(g, svc, method, rule, idx, fnName, ctxType)
	}
}

func generateUnaryOperationFn(g *protogen.GeneratedFile, svc *protogen.Service, method *protogen.Method, rule httpRule, idx int, fnName string, ctxType string) {
	g.P("// ", fnName, " builds the ", method.GoName, " endpoint.")
	g.P("func ", fnName, "(svc ", svc.GoName, "Handler) *", sym(g, "Operation"), " {")
	g.P("var op ", sym(g, "Operation"))
	g.P("return (*", sym(g, "OperationBuilder"), "[", g.QualifiedGoIdent(method.Input.GoIdent), ", ", g.QualifiedGoIdent(method.Output.GoIdent), "])(&op).")
	g.P("SetMethod(", httpMethodConst(g, rule.method), ").")
	g.P("SetPath(", pathConstName(svc.GoName, method.GoName, idx), ").")
	g.P("SetRPC(", rpcNameConstName(svc.GoName, method.GoName), ").")
	g.P("SetUnaryHandler(func(ctx ", ctxType, ", req *", sym(g, "Request"), "[", g.QualifiedGoIdent(method.Input.GoIdent), "]) (*", sym(g, "Response"), "[", g.QualifiedGoIdent(method.Output.GoIdent), "], error) {")
	g.P("resp, err := svc.", method.GoName, "(ctx, req.Msg())")
	g.P("if err != nil {")
	g.P("return nil, err")
	g.P("}")
	g.P("return ", sym(g, "NewResponse"), "(resp), nil")
	g.P("}).")
	if rule.body != "" {
		g.P("SetBody(", strconvQuote(rule.body), ").")
	}
	if rule.responseBody != "" {
		g.P("SetResponseBody(", strconvQuote(rule.responseBody), ").")
	}
	g.P("SetRequestBuilder(", requestBuilderName(svc.GoName, method.GoName, idx), ").")
	g.P("Build()")
	g.P("}")
	g.P()
}

func generateServerStreamOperationFn(g *protogen.GeneratedFile, svc *protogen.Service, method *protogen.Method, rule httpRule, idx int, fnName string, ctxType string) {
	g.P("// ", fnName, " builds the ", method.GoName, " endpoint.")
	g.P("func ", fnName, "(svc ", svc.GoName, "Handler) *", sym(g, "Operation"), " {")
	g.P("var op ", sym(g, "Operation"))
	g.P("return (*", sym(g, "OperationBuilder"), "[", g.QualifiedGoIdent(method.Input.GoIdent), ", *", g.QualifiedGoIdent(method.Output.GoIdent), "])(&op).")
	g.P("SetMethod(", httpMethodConst(g, rule.method), ").")
	g.P("SetPath(", pathConstName(svc.GoName, method.GoName, idx), ").")
	g.P("SetRPC(", rpcNameConstName(svc.GoName, method.GoName), ").")
	g.P("SetServerStreamHandler(func(ctx ", ctxType, ", req *", sym(g, "Request"), "[", g.QualifiedGoIdent(method.Input.GoIdent), "], stream ", sym(g, "ServerStream"), "[*", g.QualifiedGoIdent(method.Output.GoIdent), "]) error {")
	g.P("return svc.", method.GoName, "(ctx, req.Msg(), stream)")
	g.P("}).")
	if rule.body != "" {
		g.P("SetBody(", strconvQuote(rule.body), ").")
	}
	if rule.responseBody != "" {
		g.P("SetResponseBody(", strconvQuote(rule.responseBody), ").")
	}
	g.P("SetRequestBuilder(", requestBuilderName(svc.GoName, method.GoName, idx), ").")
	g.P("Build()")
	g.P("}")
	g.P()
}

func generateClientStreamOperationFn(g *protogen.GeneratedFile, svc *protogen.Service, method *protogen.Method, rule httpRule, idx int, fnName string, ctxType string) {
	g.P("// ", fnName, " builds the ", method.GoName, " endpoint.")
	g.P("func ", fnName, "(svc ", svc.GoName, "Handler) *", sym(g, "Operation"), " {")
	g.P("var op ", sym(g, "Operation"))
	g.P("return (*", sym(g, "OperationBuilder"), "[*", g.QualifiedGoIdent(method.Input.GoIdent), ", ", g.QualifiedGoIdent(method.Output.GoIdent), "])(&op).")
	g.P("SetMethod(", httpMethodConst(g, rule.method), ").")
	g.P("SetPath(", pathConstName(svc.GoName, method.GoName, idx), ").")
	g.P("SetRPC(", rpcNameConstName(svc.GoName, method.GoName), ").")
	g.P("SetClientStreamHandler(func(ctx ", ctxType, ", stream ", sym(g, "ClientStream"), "[*", g.QualifiedGoIdent(method.Input.GoIdent), "]) (*", sym(g, "Response"), "[", g.QualifiedGoIdent(method.Output.GoIdent), "], error) {")
	g.P("resp, err := svc.", method.GoName, "(ctx, stream)")
	g.P("if err != nil {")
	g.P("return nil, err")
	g.P("}")
	g.P("return ", sym(g, "NewResponse"), "(resp), nil")
	g.P("}).")
	if rule.responseBody != "" {
		g.P("SetResponseBody(", strconvQuote(rule.responseBody), ").")
	}
	g.P("Build()")
	g.P("}")
	g.P()
}

func generateBidiStreamOperationFn(g *protogen.GeneratedFile, svc *protogen.Service, method *protogen.Method, rule httpRule, idx int, fnName string, ctxType string) {
	g.P("// ", fnName, " builds the ", method.GoName, " endpoint.")
	g.P("func ", fnName, "(svc ", svc.GoName, "Handler) *", sym(g, "Operation"), " {")
	g.P("var op ", sym(g, "Operation"))
	g.P("return (*", sym(g, "OperationBuilder"), "[*", g.QualifiedGoIdent(method.Input.GoIdent), ", *", g.QualifiedGoIdent(method.Output.GoIdent), "])(&op).")
	g.P("SetMethod(", httpMethodConst(g, rule.method), ").")
	g.P("SetPath(", pathConstName(svc.GoName, method.GoName, idx), ").")
	g.P("SetRPC(", rpcNameConstName(svc.GoName, method.GoName), ").")
	g.P("SetBidiStreamHandler(func(ctx ", ctxType, ", stream ", sym(g, "BidiStream"), "[*", g.QualifiedGoIdent(method.Input.GoIdent), ", *", g.QualifiedGoIdent(method.Output.GoIdent), "]) error {")
	g.P("return svc.", method.GoName, "(ctx, stream)")
	g.P("}).")
	if rule.body != "" {
		g.P("SetBody(", strconvQuote(rule.body), ").")
	}
	if rule.responseBody != "" {
		g.P("SetResponseBody(", strconvQuote(rule.responseBody), ").")
	}
	g.P("Build()")
	g.P("}")
	g.P()
}
