// Generation-time validation of google.api.http bindings.
package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

// validateHTTPRules validates every HTTP binding in a proto file before any
// code is emitted. Invalid bindings fail generation with the method and rule
// named, instead of silently mis-generating.
func validateHTTPRules(file *protogen.File) error {
	for _, svc := range file.Services {
		for _, method := range svc.Methods {
			rules, err := extractHTTPRules(method)
			if err != nil {
				return fmt.Errorf("%s.%s: %v", svc.Desc.Name(), method.Desc.Name(), err)
			}
			seen := make(map[string]string)
			for _, rule := range rules {
				where := fmt.Sprintf("%s.%s (%s %s)", svc.Desc.Name(), method.Desc.Name(), rule.method, rule.path)

				// Custom verbs must be a single :verb suffix on the final
				// segment.
				if err := validateVerbPlacement(rule); err != nil {
					return fmt.Errorf("%s: %v", where, err)
				}

				// response_body must name a field of the response message.
				if rule.responseBody != "" {
					if _, err := resolveFieldPath(method.Output, rule.responseBody); err != nil {
						return fmt.Errorf("%s: response_body %q: %v", where, rule.responseBody, err)
					}
				}

				// Duplicate method+path within one method's bindings would
				// register the same route twice.
				key := rule.method + " " + rule.path
				if prev, ok := seen[key]; ok {
					return fmt.Errorf("%s: duplicate binding %s (also declared by %s)", where, key, prev)
				}
				seen[key] = where
			}
		}
	}
	return nil
}

// validateVerbPlacement checks that a path template's custom verb, if any,
// is a single :verb suffix on the final segment. The runtime parser rejects
// some malformed verbs at registration time; this catches them at generation
// time with the rule named.
func validateVerbPlacement(rule httpRule) error {
	colon := findVerbColon(rule.path)
	if colon < 0 {
		return nil
	}
	if strings.Contains(rule.path[colon+1:], "/") {
		return fmt.Errorf("custom verb must be the final segment of the path")
	}
	verb := rule.path[colon+1:]
	if verb == "" {
		return fmt.Errorf("empty custom verb")
	}
	if strings.Contains(verb, ":") {
		return fmt.Errorf("multiple custom verbs")
	}
	if strings.Contains(verb, "{") || strings.Contains(verb, "}") {
		return fmt.Errorf("custom verb must not contain braces")
	}
	return nil
}

// findVerbColon returns the index of the custom-verb colon in a path
// template, or -1 when the path has no custom verb. Colons inside braces
// (resource name expressions like {name=users/*}) are not verbs.
func findVerbColon(path string) int {
	depth := 0
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ':':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}