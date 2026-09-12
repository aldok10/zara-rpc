// Message population from HTTP inputs.
package runtime

import (
	"encoding/base64"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/status"
)

// ---------------------------------------------------------------------------
// Message population (server side)
// ---------------------------------------------------------------------------

// PopulateMessage fills msg from path params, query params, and body,
// mirroring grpc-gateway's google.api.http field distribution rules:
//
//   - Fields named in path params are extracted from the URL path.
//   - Fields named by bodyField are decoded from the request body.
//   - All remaining fields are populated from query parameters.
//
// bodyField == "*" decodes the entire body into msg and skips query
// population (matching grpc-gateway semantics).
//
// body holds the raw request body bytes, buffered by the mux for unary and
// server-streaming requests.
func PopulateMessage(msg any, params map[string]string, query url.Values, body []byte, bodyField string, c encoding.Codec) error {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return status.NewErrorf(codes.CodeInternal, "populate: expected non-nil pointer, got %T", msg)
	}

	// 1. Path parameters. Unknown path params are an error.
	for name, value := range params {
		if err := setFieldString(v, name, value, true); err != nil {
			return err
		}
	}

	// 2. Body.
	if bodyField != "" {
		if bodyField == "*" {
			if err := c.Unmarshal(body, msg); err != nil {
				return status.NewErrorf(codes.CodeInvalidArgument, "decode request body: %v", err)
			}
			return nil
		}
		if err := SetFieldFromBody(v, bodyField, body, c); err != nil {
			return err
		}
	}

	// 3. Query parameters. Unknown query params are ignored (grpc-gateway
	// behavior), and fields already bound to path/body are skipped.
	for name, values := range query {
		if _, ok := params[name]; ok {
			continue
		}
		if name == bodyField {
			continue
		}
		if err := setFieldStrings(v, name, values, false); err != nil {
			return err
		}
	}

	return nil
}

// PopulateQuery fills msg from query parameters, skipping fields named in
// bound. Used by generated request builders after path params and body
// have been extracted type-safely. The variadic bound list avoids a
// per-request map allocation.
func PopulateQuery(msg any, query url.Values, bound ...string) error {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return status.NewErrorf(codes.CodeInternal, "populate: expected non-nil pointer, got %T", msg)
	}
	for name, values := range query {
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
		if err := setFieldStrings(v, name, values, false); err != nil {
			return err
		}
	}
	return nil
}

// SetFieldFromBody decodes the body into the named field of msg.
func SetFieldFromBody(msg reflect.Value, name string, body []byte, c encoding.Codec) error {
	f, ok := FindField(msg, name)
	if !ok {
		return status.NewErrorf(codes.CodeInvalidArgument, "body field %q not found in %s", name, msg.Type())
	}
	// Allocate nil pointers so Decode has a target.
	if f.Kind() == reflect.Ptr && f.IsNil() {
		f.Set(reflect.New(f.Type().Elem()))
	}
	target := f
	if target.Kind() != reflect.Ptr {
		if !target.CanAddr() {
			return status.NewErrorf(codes.CodeInternal, "body field %q is not addressable", name)
		}
		target = target.Addr()
	}
	if err := c.Unmarshal(body, target.Interface()); err != nil {
		return status.NewErrorf(codes.CodeInvalidArgument, "decode request body into %q: %v", name, err)
	}
	return nil
}

// setFieldString sets a field from a single string value (path params).
// strict controls whether unknown fields are errors (true for path params,
// false for query params). It is deliberately typed (not any): boxing a
// string into an interface allocates, and this runs once per path param
// per request.
func setFieldString(root reflect.Value, name, value string, strict bool) error {
	f, ok := findFieldPath(root, name)
	if !ok {
		if strict {
			return status.NewErrorf(codes.CodeInvalidArgument, "field %q not found in %s", name, root.Type())
		}
		return nil
	}
	return setStringValue(f, value)
}

// setFieldStrings sets a field from a []string value (query params). As
// with setFieldString, the typed signature avoids boxing the slice into an
// interface.
func setFieldStrings(root reflect.Value, name string, values []string, strict bool) error {
	if len(values) == 0 {
		return nil
	}
	f, ok := findFieldPath(root, name)
	if !ok {
		if strict {
			return status.NewErrorf(codes.CodeInvalidArgument, "field %q not found in %s", name, root.Type())
		}
		return nil
	}
	if f.Kind() == reflect.Slice {
		slice := reflect.MakeSlice(f.Type(), 0, len(values))
		for _, s := range values {
			elem := reflect.New(f.Type().Elem()).Elem()
			if err := setStringValue(elem, s); err != nil {
				return err
			}
			slice = reflect.Append(slice, elem)
		}
		f.Set(slice)
		return nil
	}
	return setStringValue(f, values[len(values)-1])
}

// findFieldPath finds a struct field by name, resolving dotted paths
// (e.g. "address.city").
func findFieldPath(root reflect.Value, name string) (reflect.Value, bool) {
	if !strings.Contains(name, ".") {
		return FindField(root, name)
	}
	parts := strings.Split(name, ".")
	cur := root
	for i, part := range parts {
		f, ok := FindField(cur, part)
		if !ok {
			return reflect.Value{}, false
		}
		if i == len(parts)-1 {
			return f, true
		}
		cur = f
	}
	return reflect.Value{}, false
}

// fieldCacheKey identifies a FindField lookup: the message type and the
// field name being resolved. reflect.Type is immutable at runtime, so a
// cached result can never go stale.
type fieldCacheKey struct {
	typ  reflect.Type
	name string
}

// fieldCache stores the resolved field index per (type, name). A negative
// index means "not found" — the miss is cached too, so repeated lookups of
// unknown fields (e.g. a query param that matches no struct field) skip
// the scan as well. A plain map + RWMutex beats sync.Map here: the read
// path is a typed map lookup under RLock (~23ns) versus sync.Map's
// interface boxing + type assertion (~34ns), measured on the cache-hit
// path. Writes happen once per type (first request), so the write lock is
// cold.
var (
	fieldCacheMu sync.RWMutex
	fieldCache   = make(map[fieldCacheKey]int)
)

// FindField finds a struct field by json tag name or case-insensitive field
// name, allocating nil pointers along the way. The field-index lookup is
// cached per (type, name): the reflection scan (StructField copy + tag
// parse) runs once per type, not once per request.
func FindField(v reflect.Value, name string) (reflect.Value, bool) {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			if !v.CanSet() {
				return reflect.Value{}, false
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	t := v.Type()
	key := fieldCacheKey{t, name}
	fieldCacheMu.RLock()
	idx, ok := fieldCache[key]
	fieldCacheMu.RUnlock()
	if ok {
		if idx < 0 {
			return reflect.Value{}, false
		}
		return v.Field(idx), true
	}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue // unexported
		}
		// Zero-alloc json tag lookup: strip the ",omitempty" suffix
		// without building a slice.
		tag := sf.Tag.Get("json")
		if j := strings.IndexByte(tag, ','); j >= 0 {
			tag = tag[:j]
		}
		if tag == name || (tag == "" && strings.EqualFold(sf.Name, name)) {
			fieldCacheMu.Lock()
			fieldCache[key] = i
			fieldCacheMu.Unlock()
			return v.Field(i), true
		}
	}
	fieldCacheMu.Lock()
	fieldCache[key] = -1
	fieldCacheMu.Unlock()
	return reflect.Value{}, false
}

// setStringValue converts a string to the field's type and sets it.
func setStringValue(f reflect.Value, s string) error {
	for f.Kind() == reflect.Ptr {
		if f.IsNil() {
			f.Set(reflect.New(f.Type().Elem()))
		}
		f = f.Elem()
	}

	switch f.Kind() {
	case reflect.String:
		f.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return status.NewErrorf(codes.CodeInvalidArgument, "parse %q as %s: %v", s, f.Type(), err)
		}
		f.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return status.NewErrorf(codes.CodeInvalidArgument, "parse %q as %s: %v", s, f.Type(), err)
		}
		f.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return status.NewErrorf(codes.CodeInvalidArgument, "parse %q as %s: %v", s, f.Type(), err)
		}
		f.SetFloat(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return status.NewErrorf(codes.CodeInvalidArgument, "parse %q as bool: %v", s, err)
		}
		f.SetBool(b)
	case reflect.Slice:
		if f.Type().Elem().Kind() == reflect.Uint8 {
			// []byte: base64 decode.
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return status.NewErrorf(codes.CodeInvalidArgument, "decode %q as base64: %v", s, err)
			}
			f.SetBytes(b)
			return nil
		}
		return status.NewErrorf(codes.CodeInvalidArgument, "unsupported slice field type %s", f.Type())
	case reflect.Struct:
		if f.Type() == reflect.TypeOf(time.Time{}) {
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return status.NewErrorf(codes.CodeInvalidArgument, "parse %q as time: %v", s, err)
			}
			f.Set(reflect.ValueOf(t))
			return nil
		}
		// google.protobuf.Timestamp: struct with Seconds/Nanos fields.
		if isProtoTimestamp(f.Type()) {
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return status.NewErrorf(codes.CodeInvalidArgument, "parse %q as timestamp: %v", s, err)
			}
			sec := f.FieldByName("Seconds")
			nanos := f.FieldByName("Nanos")
			if sec.CanSet() {
				sec.SetInt(t.Unix())
			}
			if nanos.CanSet() {
				nanos.SetInt(int64(t.Nanosecond()))
			}
			return nil
		}
		return status.NewErrorf(codes.CodeInvalidArgument, "unsupported struct field type %s", f.Type())
	default:
		return status.NewErrorf(codes.CodeInvalidArgument, "unsupported field type %s", f.Type())
	}
	return nil
}

// isProtoTimestamp reports whether t looks like google.protobuf.Timestamp
// (a struct with Seconds and Nanos fields).
func isProtoTimestamp(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	sec, ok1 := t.FieldByName("Seconds")
	nanos, ok2 := t.FieldByName("Nanos")
	return ok1 && ok2 &&
		sec.Type.Kind() == reflect.Int64 &&
		nanos.Type.Kind() == reflect.Int32
}
