// protoc-gen-zararpc generates Go code that registers HTTP operations
// from google.api.http annotations, in the style of protoc-gen-grpc-gateway,
// but targeting the zararpc runtime instead of gRPC.
//
// The plugin is a superset of protoc-gen-go: it emits the .pb.go files
// itself (via protobuf-go's internal generator), applies the custom struct
// tags declared with (zara.options.tags)/(zara.options.oneof_tags) directly
// to the generated structs, and then emits the zararpc registration
// (.zararpc.go) and REST->gRPC gateway (.gateway.go) files. No separate
// protoc-gen-go or zararpc-tags step is needed.
//
// Usage:
//
//	protoc --zararpc_out=. path/to/service.proto
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"

	// protoc-gen-go's generator engine, used to emit the .pb.go files.
	"google.golang.org/protobuf/cmd/protoc-gen-go/internal_gengo"
)

func main() {
	var flags flag.FlagSet
	opts := protogen.Options{ParamFunc: flags.Set}

	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "protoc-gen-zararpc: %v\n", err)
		os.Exit(1)
	}
	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(in, req); err != nil {
		fmt.Fprintf(os.Stderr, "protoc-gen-zararpc: %v\n", err)
		os.Exit(1)
	}

	gen, err := opts.New(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "protoc-gen-zararpc: %v\n", err)
		os.Exit(1)
	}
	gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

	if err := generatePBGoFiles(gen, req); err != nil {
		gen.Error(err)
	}
	// The gRPC bridge helpers (grpcCtxFor, zaraToGRPC, stream adapters) live in
	// the framework's grpcbridge package, so generated output stays thin.
	for _, f := range gen.Files {
		if !f.Generate {
			continue
		}
		if len(f.Services) == 0 {
			continue
		}
		// Fail fast on invalid HTTP bindings before emitting anything.
		if err := validateHTTPRules(f); err != nil {
			gen.Error(err)
			continue
		}
		generateFile(gen, f)
		generateGatewayFile(gen, f)
		generateGRPCFile(gen, f)
	}

	resp := gen.Response()
	out, err := proto.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "protoc-gen-zararpc: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "protoc-gen-zararpc: %v\n", err)
		os.Exit(1)
	}
}

// generatePBGoFiles runs protoc-gen-go's engine on the same request and
// emits the .pb.go files through gen, patching custom struct tags into the
// generated structs. The generator owns the .pb.go output so the custom tag
// overlay (.tags.go) and the separate zararpc-tags tool are unnecessary.
func generatePBGoFiles(gen *protogen.Plugin, req *pluginpb.CodeGeneratorRequest) error {
	pbgo, err := protogen.Options{}.New(req)
	if err != nil {
		return err
	}
	for _, f := range pbgo.Files {
		if !f.Generate {
			continue
		}
		g := internal_gengo.GenerateFile(pbgo, f)
		content, err := g.Content()
		if err != nil {
			return fmt.Errorf("%s: %v", f.Desc.Path(), err)
		}
		if overlay := collectTags(f); len(overlay) > 0 {
			content, err = patchTags(content, overlay)
			if err != nil {
				return fmt.Errorf("%s: patch custom tags: %v", f.Desc.Path(), err)
			}
		}
		filename := f.GeneratedFilenamePrefix + ".pb.go"
		gf := gen.NewGeneratedFile(filename, f.GoImportPath)
		gf.P(string(content))
	}
	return nil
}

// httpRule is a parsed google.api.http annotation.
type httpRule struct {
	method       string
	path         string
	body         string
	responseBody string
}

// extractHTTPRules returns the HTTP rules for a method, including
// additional_bindings.
func extractHTTPRules(method *protogen.Method) ([]httpRule, error) {
	if method.Desc.Options() == nil {
		return nil, nil
	}
	opts, ok := method.Desc.Options().(*descriptorpb.MethodOptions)
	if !ok {
		return nil, nil
	}
	if !proto.HasExtension(opts, annotations.E_Http) {
		return nil, nil
	}
	ext := proto.GetExtension(opts, annotations.E_Http)
	rule, ok := ext.(*annotations.HttpRule)
	if !ok {
		return nil, fmt.Errorf("extension is %T; want *annotations.HttpRule", ext)
	}

	var rules []httpRule
	rules = append(rules, ruleFromHTTPRule(rule))
	for _, additional := range rule.GetAdditionalBindings() {
		rules = append(rules, ruleFromHTTPRule(additional))
	}
	return rules, nil
}

func ruleFromHTTPRule(rule *annotations.HttpRule) httpRule {
	switch pattern := rule.Pattern.(type) {
	case *annotations.HttpRule_Get:
		return httpRule{method: "GET", path: pattern.Get, body: rule.GetBody(), responseBody: rule.GetResponseBody()}
	case *annotations.HttpRule_Put:
		return httpRule{method: "PUT", path: pattern.Put, body: rule.GetBody(), responseBody: rule.GetResponseBody()}
	case *annotations.HttpRule_Post:
		return httpRule{method: "POST", path: pattern.Post, body: rule.GetBody(), responseBody: rule.GetResponseBody()}
	case *annotations.HttpRule_Delete:
		return httpRule{method: "DELETE", path: pattern.Delete, body: rule.GetBody(), responseBody: rule.GetResponseBody()}
	case *annotations.HttpRule_Patch:
		return httpRule{method: "PATCH", path: pattern.Patch, body: rule.GetBody(), responseBody: rule.GetResponseBody()}
	case *annotations.HttpRule_Custom:
		return httpRule{method: pattern.Custom.GetKind(), path: pattern.Custom.GetPath(), body: rule.GetBody(), responseBody: rule.GetResponseBody()}
	default:
		return httpRule{}
	}
}