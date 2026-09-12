package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"sort"
	"strconv"

	"github.com/fatih/structtag"
)

// tagOverlay maps Go struct name -> Go field name -> struct tag string,
// collected from the (zara.options.tags) field option and the
// (zara.options.oneof_tags) oneof option.
type tagOverlay map[string]map[string]string

// patchTags rewrites the struct tags in a generated .pb.go file according
// to the overlay and returns the formatted result. The .pb.go is generated
// by protoc-gen-go's engine, so the custom tags cannot be emitted directly;
// patching the generated source is the only way to land them on the structs
// (reflection-based libraries like encoding/xml, gorm, and mapstructure
// read struct tags).
func patchTags(content []byte, overlay tagOverlay) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	patched := 0
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			fields, ok := overlay[ts.Name.Name]
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					continue // embedded
				}
				for _, name := range field.Names {
					tag, ok := fields[name.Name]
					if !ok {
						continue
					}
					merged, err := mergeTags(tagValue(field.Tag), tag)
					if err != nil {
						return nil, fmt.Errorf("field %s.%s: %v", ts.Name.Name, name.Name, err)
					}
					field.Tag = &ast.BasicLit{
						ValuePos: field.Pos(),
						Kind:     token.STRING,
						Value:    goString(merged),
					}
					patched++
				}
			}
		}
	}
	if patched == 0 {
		return nil, fmt.Errorf("no matching struct fields (stale overlay?)")
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// tagValue returns the unquoted value of a struct tag literal, or "".
func tagValue(tag *ast.BasicLit) string {
	if tag == nil {
		return ""
	}
	s, err := strconv.Unquote(tag.Value)
	if err != nil {
		return ""
	}
	return s
}

// mergeTags merges the overlay tag string into the existing struct tag
// string. Existing keys are replaced in place; new keys are appended in
// sorted order. Parsing and rendering go through fatih/structtag, which
// understands the full Go tag grammar (quoted values, options, escaped
// quotes).
func mergeTags(existing, overlay string) (string, error) {
	oldTags, err := structtag.Parse(existing)
	if err != nil {
		return "", fmt.Errorf("parse existing tag %q: %v", existing, err)
	}
	newTags, err := structtag.Parse(overlay)
	if err != nil {
		return "", fmt.Errorf("parse overlay tag %q: %v", overlay, err)
	}
	sort.Stable(newTags)
	for _, t := range newTags.Tags() {
		if err := oldTags.Set(t); err != nil {
			return "", err
		}
	}
	return oldTags.String(), nil
}