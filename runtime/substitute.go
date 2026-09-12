// Client-side path substitution.
package runtime

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/status"
)

// ---------------------------------------------------------------------------
// Client-side path substitution
// ---------------------------------------------------------------------------

// SubstitutePathParams replaces {name} segments in a path template with
// field values from req. It is a thin wrapper around
// SubstitutePathParamsBound for callers that do not need the bound
// parameter names.
func SubstitutePathParams(template string, req any) (string, error) {
	path, _, err := SubstitutePathParamsBound(template, req)
	return path, err
}

// SubstitutePathParamsBound replaces {name} segments in a path template
// with field values from req. It also returns the names of the path
// parameters it substituted, in template order, so callers can skip those
// fields when converting the remaining message to query parameters. The
// returned slice is nil when the template has no parameters, keeping the
// common no-param case allocation-free.
func SubstitutePathParamsBound(template string, req any) (string, []string, error) {
	if req == nil {
		return template, nil, nil
	}
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return template, nil, nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return template, nil, nil
	}
	t := v.Type()

	// Fast path: no braces means no substitution and no bound names.
	if strings.IndexByte(template, '{') < 0 {
		return template, nil, nil
	}

	var sb strings.Builder
	// The result is usually no longer than the template (parameter values
	// replace their {name} segments), so pre-sizing avoids a regrow.
	sb.Grow(len(template))
	var bound []string
	rest := template
	for {
		start := strings.Index(rest, "{")
		if start < 0 {
			sb.WriteString(rest)
			break
		}
		sb.WriteString(rest[:start])
		end := strings.Index(rest[start:], "}")
		if end < 0 {
			return "", nil, fmt.Errorf("unclosed brace in %q", template)
		}
		inner := rest[start+1 : start+end]
		name := inner
		if eq := strings.Index(inner, "="); eq >= 0 {
			name = inner[:eq]
		}

		val, ok := fieldValueByName(v, t, name)
		if !ok {
			return "", nil, status.NewErrorf(codes.CodeInvalidArgument, "path parameter %q not found in %s", name, t)
		}
		// Fast path: path params are usually strings; Sprintf would
		// allocate a fresh buffer for the same bytes.
		if val.Kind() == reflect.String {
			sb.WriteString(val.String())
		} else {
			sb.WriteString(reflectValueToString(val))
		}
		bound = append(bound, name)
		rest = rest[start+end+1:]
	}
	return sb.String(), bound, nil
}

// fieldValueByName finds a struct field by protojson name (json tag) or Go
// name and returns its value. The reflect.Value is returned instead of an
// interface so string fields can be read without boxing.
func fieldValueByName(v reflect.Value, t reflect.Type, name string) (reflect.Value, bool) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue // unexported
		}
		if ProtoName(sf) == name || sf.Name == name {
			return v.Field(i), true
		}
	}
	return reflect.Value{}, false
}

// reflectValueToString renders a non-string reflect.Value for path/query
// substitution without fmt.Sprintf: numeric and bool kinds use strconv,
// which avoids the interface boxing and buffer allocation that fmt does.
// The fallback keeps fmt for exotic kinds (slices, maps, structs).
func reflectValueToString(fv reflect.Value) string {
	switch fv.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(fv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(fv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(fv.Uint(), 10)
	case reflect.Float32:
		return strconv.FormatFloat(fv.Float(), 'g', -1, 32)
	case reflect.Float64:
		return strconv.FormatFloat(fv.Float(), 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", fv.Interface())
	}
}

// MessageToQuery converts the exported fields of a request message into
// query parameters, using the protojson name (json tag) as the key. Fields
// named in bound (path parameters already substituted into the URL) are
// skipped, mirroring the server-side rule that path-bound fields are not
// query-populated. The variadic bound list avoids a per-call map
// allocation.
func MessageToQuery(req any, bound ...string) (url.Values, error) {
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return url.Values{}, nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return url.Values{}, nil
	}
	t := v.Type()
	// Lazily allocate the values map: when every field is path-bound (or
	// the message is empty) nothing is added and the nil map encodes to an
	// empty query without allocating.
	var q url.Values
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue // unexported (protobuf internal state)
		}
		name := ProtoName(sf)
		skip := false
		for _, b := range bound {
			if name == b {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		fv := v.Field(i)
		if fv.Kind() == reflect.Ptr {
			if fv.IsNil() {
				continue
			}
			fv = fv.Elem()
		}
		if q == nil {
			q = url.Values{}
		}
		if fv.Kind() == reflect.Slice {
			for j := 0; j < fv.Len(); j++ {
				elem := fv.Index(j)
				if elem.Kind() == reflect.String {
					q.Add(name, elem.String())
				} else {
					q.Add(name, reflectValueToString(elem))
				}
			}
			continue
		}
		if fv.Kind() == reflect.String {
			q.Set(name, fv.String())
		} else {
			q.Set(name, reflectValueToString(fv))
		}
	}
	return q, nil
}

// ExtractFieldValue returns the value of a named field of a request
// message, matching by protojson name or Go name.
func ExtractFieldValue(req any, name string) (any, error) {
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, status.NewErrorf(codes.CodeInvalidArgument, "field %q not found in nil request", name)
		}
		v = v.Elem()
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		if ProtoName(sf) == name || sf.Name == name {
			return v.Field(i).Interface(), nil
		}
	}
	return nil, status.NewErrorf(codes.CodeInvalidArgument, "field %q not found in %s", name, t)
}

// ProtoName returns the protojson name of a struct field (the json tag).
func ProtoName(sf reflect.StructField) string {
	name, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
	if name == "" || name == "-" {
		return sf.Name
	}
	return name
}
