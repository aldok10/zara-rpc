package status

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/metadata"
)

// Error is an RPC error with a code, message, optional HTTP headers, and
// optional protobuf Any details. It implements the error interface and is
// compatible with errors.Is/errors.As.
type Error struct {
	code    codes.Code
	message string
	err     error
	meta    http.Header
	details []*anypb.Any
}

// NewError creates a new Error with the given code and underlying error.
func NewError(code codes.Code, err error) *Error {
	return &Error{
		code: code,
		err:  err,
	}
}

// NewErrorf creates a new Error with a formatted message.
func NewErrorf(code codes.Code, format string, args ...any) *Error {
	return &Error{
		code:    code,
		message: fmt.Sprintf(format, args...),
	}
}

// Code returns the error code.
func (e *Error) Code() codes.Code {
	return e.code
}

// Message returns the error message.
func (e *Error) Message() string {
	if e.message != "" {
		return e.message
	}
	if e.err != nil {
		return e.err.Error()
	}
	return e.code.String()
}

// Error implements the error interface.
func (e *Error) Error() string {
	msg := e.Message()
	return fmt.Sprintf("rpc error: %s: %s", e.code, msg)
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.err
}

// Meta returns the mutable HTTP header for error metadata.
func (e *Error) Meta() http.Header {
	if e.meta == nil {
		e.meta = make(http.Header)
	}
	return e.meta
}

// WithDetails appends protobuf Any detail payloads to the error and
// returns the error for chaining. Details are serialized as
// google.rpc.Status on the wire (grpc-status-details-bin) so gRPC-aware
// clients recover them.
func (e *Error) WithDetails(details ...*anypb.Any) *Error {
	e.details = append(e.details, details...)
	return e
}

// Details returns the protobuf Any detail payloads attached to the error.
func (e *Error) Details() []*anypb.Any {
	return e.details
}

// GRPCStatus returns a grpc-compatible status view of the error. The
// returned *GRPCStatus has the same method shape as grpc-go's
// *status.Status (Code, Message, Details, Proto), so the reflection-based
// FromGRPCStatus and grpc-go interop both work without importing grpc.
func (e *Error) GRPCStatus() *GRPCStatus {
	return &GRPCStatus{code: e.code, message: e.Message(), details: e.details}
}

// MarshalStatus serializes the error as a google.rpc.Status message
// (code=1 varint, message=2 string, details=3 repeated message). The wire
// format is hand-rolled so the root module stays grpc-free and does not
// register google/rpc/status.proto (grpc-go's status package registers it,
// and a duplicate registration would panic in the examples process).
func (e *Error) MarshalStatus() ([]byte, error) {
	return marshalStatus(&Status{Code: int32(e.code), Message: e.Message(), Details: e.details})
}

// GRPCStatus is a grpc-compatible status view of an RPC error. It mirrors
// the method shape of grpc-go's *status.Status so reflection-based
// conversion works in both directions.
type GRPCStatus struct {
	code    codes.Code
	message string
	details []*anypb.Any
}

// Code returns the status code.
func (s *GRPCStatus) Code() codes.Code { return s.code }

// Message returns the status message.
func (s *GRPCStatus) Message() string { return s.message }

// Details returns the protobuf Any detail payloads.
func (s *GRPCStatus) Details() []*anypb.Any { return s.details }

// Proto returns the plain google.rpc.Status representation.
func (s *GRPCStatus) Proto() *Status {
	return &Status{Code: int32(s.code), Message: s.message, Details: s.details}
}

// Status is the plain google.rpc.Status representation: code, message, and
// protobuf Any details. It is not a registered proto message (see
// MarshalStatus for why); it exists so GRPCStatus.Proto and the
// reflection-based FromGRPCStatus have a concrete GetDetails target.
type Status struct {
	Code    int32
	Message string
	Details []*anypb.Any
}

// GetCode returns the status code.
func (s *Status) GetCode() int32 { return s.Code }

// GetMessage returns the status message.
func (s *Status) GetMessage() string { return s.Message }

// GetDetails returns the protobuf Any detail payloads.
func (s *Status) GetDetails() []*anypb.Any { return s.Details }

// marshalStatus serializes a Status to the google.rpc.Status wire format.
func marshalStatus(st *Status) ([]byte, error) {
	b := protowire.AppendTag(nil, 1, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(st.Code))
	b = protowire.AppendTag(b, 2, protowire.BytesType)
	b = protowire.AppendString(b, st.Message)
	for _, d := range st.Details {
		data, err := proto.Marshal(d)
		if err != nil {
			return nil, err
		}
		b = protowire.AppendTag(b, 3, protowire.BytesType)
		b = protowire.AppendBytes(b, data)
	}
	return b, nil
}

// unmarshalStatus parses a google.rpc.Status wire message.
func unmarshalStatus(data []byte) (*Status, error) {
	st := &Status{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			st.Code = int32(v)
			data = data[n:]
		case num == 2 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			st.Message = string(v)
			data = data[n:]
		case num == 3 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			any := &anypb.Any{}
			if err := proto.Unmarshal(v, any); err != nil {
				return nil, err
			}
			st.Details = append(st.Details, any)
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			data = data[n:]
		}
	}
	return st, nil
}

// HTTPStatus returns the HTTP status code for this error.
func (e *Error) HTTPStatus() int {
	return e.code.HTTPStatus()
}

// Code extracts the Code from an error chain. Returns codes.CodeUnknown if
// no *Error is found. It mirrors grpc-go's status.Code.
func Code(err error) codes.Code {
	var rpcErr *Error
	if errors.As(err, &rpcErr) {
		return rpcErr.code
	}
	return codes.CodeUnknown
}

// FromError extracts an *Error from an error chain. It returns (nil, false)
// when err is not an *Error. It mirrors grpc-go's status.FromError.
func FromError(err error) (*Error, bool) {
	var rpcErr *Error
	if errors.As(err, &rpcErr) {
		return rpcErr, true
	}
	return nil, false
}

// FromHTTP parses an error response body into an *Error. It is used by
// generated clients to convert non-2xx responses into RPC errors. When the
// response carries a grpc-status-details-bin header (base64 google.rpc.Status),
// the code, message, and detail payloads from the header win over the JSON
// body.
func FromHTTP(resp *http.Response) error {
	code := codes.CodeUnknown
	message := resp.Status
	var details []*anypb.Any

	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
		if c, ok := codes.CodeFromString(body.Code); ok {
			code = c
		}
		if body.Message != "" {
			message = body.Message
		}
	}

	if v := resp.Header.Get(metadata.HeaderGrpcStatusDetails); v != "" {
		if data, err := base64.StdEncoding.DecodeString(v); err == nil {
			if st, err := unmarshalStatus(data); err == nil {
				if st.Code != 0 {
					code = codes.Code(st.Code)
				}
				if st.Message != "" {
					message = st.Message
				}
				details = st.Details
			}
		}
	}

	return &Error{
		code:    code,
		message: message,
		meta:    resp.Header.Clone(),
		details: details,
	}
}

// FromGRPCStatus converts a google.golang.org/grpc/status error to a
// zararpc *Error so error codes and HTTP status codes survive the gateway
// hop. The root module is grpc-free, so the conversion uses reflection over
// the GRPCStatus() method instead of importing grpc. zararpc codes are
// numerically identical to grpc codes (0-16), so the conversion is a direct
// cast. Errors without a GRPCStatus method are returned unchanged.
//
// The reflection-based lookup is exercised against real grpc errors in the
// examples module (which imports grpc); the root-module test uses a fake
// status type with the same method shape.
func FromGRPCStatus(err error) error {
	if err == nil {
		return nil
	}
	for e := err; e != nil; e = errors.Unwrap(e) {
		v := reflect.ValueOf(e)
		if v.Kind() == reflect.Pointer && v.IsNil() {
			continue
		}
		m := v.MethodByName("GRPCStatus")
		if !m.IsValid() {
			continue
		}
		out := m.Call(nil)
		if len(out) != 1 || out[0].Kind() != reflect.Pointer || out[0].IsNil() {
			return err
		}
		st := out[0]
		codeM := st.MethodByName("Code")
		msgM := st.MethodByName("Message")
		if !codeM.IsValid() || !msgM.IsValid() {
			return err
		}
		code := reflectInt(codeM.Call(nil)[0])
		msg := msgM.Call(nil)[0].String()
		rpcErr := NewErrorf(codes.Code(code), "%s", msg)
		// Extract detail payloads via Proto().GetDetails() when the status
		// exposes them (grpc-go's *status.Status and our *GRPCStatus both
		// do). The details are []*anypb.Any; the concrete element type is
		// asserted so a status with a different details type degrades to
		// code+message only.
		if protoM := st.MethodByName("Proto"); protoM.IsValid() {
			protoOut := protoM.Call(nil)
			if len(protoOut) == 1 && protoOut[0].Kind() == reflect.Pointer && !protoOut[0].IsNil() {
				if detailsM := protoOut[0].MethodByName("GetDetails"); detailsM.IsValid() {
					detailsOut := detailsM.Call(nil)
					if len(detailsOut) == 1 && detailsOut[0].Kind() == reflect.Slice {
						for i := 0; i < detailsOut[0].Len(); i++ {
							item := detailsOut[0].Index(i)
							if item.Kind() == reflect.Pointer && !item.IsNil() {
								if any, ok := item.Interface().(*anypb.Any); ok {
									rpcErr.details = append(rpcErr.details, any)
								}
							}
						}
					}
				}
			}
		}
		return rpcErr
	}
	return err
}

// reflectInt extracts an int64 from a reflect.Value of any integer kind.
// grpc's codes.Code is a uint32-based type; reflect.Value.Int panics on
// unsigned values, so the conversion must branch on the kind.
func reflectInt(v reflect.Value) int64 {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(v.Uint())
	default:
		return 0
	}
}

func TestErrorDetailsRoundTrip(t *testing.T) {
	detail, err := anypb.New(structpb.NewStringValue("quota exceeded"))
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	rpcErr := NewErrorf(codes.CodeResourceExhausted, "out of quota").WithDetails(detail)

	if got := rpcErr.Details(); len(got) != 1 || got[0] != detail {
		t.Fatalf("Details() = %v, want [detail]", got)
	}

	// MarshalStatus produces a google.rpc.Status wire message.
	data, err := rpcErr.MarshalStatus()
	if err != nil {
		t.Fatalf("MarshalStatus: %v", err)
	}
	st, err := unmarshalStatus(data)
	if err != nil {
		t.Fatalf("unmarshalStatus: %v", err)
	}
	if st.Code != int32(codes.CodeResourceExhausted) {
		t.Errorf("code = %d, want %d", st.Code, codes.CodeResourceExhausted)
	}
	if st.Message != "out of quota" {
		t.Errorf("message = %q, want out of quota", st.Message)
	}
	if len(st.Details) != 1 || !proto.Equal(st.Details[0], detail) {
		t.Errorf("details = %v, want [detail]", st.Details)
	}
}

func TestFromHTTPDetails(t *testing.T) {
	detail, err := anypb.New(structpb.NewStringValue("quota exceeded"))
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	rpcErr := NewErrorf(codes.CodeResourceExhausted, "out of quota").WithDetails(detail)
	data, err := rpcErr.MarshalStatus()
	if err != nil {
		t.Fatalf("MarshalStatus: %v", err)
	}

	// Simulate a non-2xx response carrying grpc-status-details-bin.
	body := `{"code":"resource_exhausted","message":"out of quota"}`
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Header: http.Header{
			metadata.HeaderGrpcStatusDetails: {base64.StdEncoding.EncodeToString(data)},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}

	got := FromHTTP(resp)
	rpcGot, ok := FromError(got)
	if !ok {
		t.Fatalf("FromError = %v, want *Error", got)
	}
	if rpcGot.Code() != codes.CodeResourceExhausted {
		t.Errorf("code = %v, want resource_exhausted", rpcGot.Code())
	}
	if rpcGot.Message() != "out of quota" {
		t.Errorf("message = %q, want out of quota", rpcGot.Message())
	}
	if len(rpcGot.Details()) != 1 || !proto.Equal(rpcGot.Details()[0], detail) {
		t.Errorf("details = %v, want [detail]", rpcGot.Details())
	}
}

// fakeGRPCStatusWithDetails mirrors grpc's *status.Status including the
// Proto() method that carries details.
type fakeGRPCStatusWithDetails struct {
	code    uint32
	message string
	details []*anypb.Any
}

func (s *fakeGRPCStatusWithDetails) Code() uint32    { return s.code }
func (s *fakeGRPCStatusWithDetails) Message() string { return s.message }
func (s *fakeGRPCStatusWithDetails) Proto() *Status {
	return &Status{Code: int32(s.code), Message: s.message, Details: s.details}
}

type fakeGRPCErrorWithDetails struct {
	st *fakeGRPCStatusWithDetails
}

func (e *fakeGRPCErrorWithDetails) Error() string                          { return "fake grpc error" }
func (e *fakeGRPCErrorWithDetails) GRPCStatus() *fakeGRPCStatusWithDetails { return e.st }

func TestFromGRPCStatusDetails(t *testing.T) {
	detail, err := anypb.New(structpb.NewStringValue("quota exceeded"))
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	err = &fakeGRPCErrorWithDetails{st: &fakeGRPCStatusWithDetails{
		code:    8,
		message: "out of quota",
		details: []*anypb.Any{detail},
	}}
	got := FromGRPCStatus(err)
	rpcGot, ok := FromError(got)
	if !ok {
		t.Fatalf("FromError = %v, want *Error", got)
	}
	if rpcGot.Code() != codes.CodeResourceExhausted {
		t.Errorf("code = %v, want resource_exhausted", rpcGot.Code())
	}
	if len(rpcGot.Details()) != 1 || !proto.Equal(rpcGot.Details()[0], detail) {
		t.Errorf("details = %v, want [detail]", rpcGot.Details())
	}
}

func TestGRPCStatusMethod(t *testing.T) {
	detail, err := anypb.New(structpb.NewStringValue("x"))
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	rpcErr := NewErrorf(codes.CodeNotFound, "missing").WithDetails(detail)
	gs := rpcErr.GRPCStatus()
	if gs.Code() != codes.CodeNotFound || gs.Message() != "missing" {
		t.Errorf("GRPCStatus = (%v, %q)", gs.Code(), gs.Message())
	}
	if len(gs.Details()) != 1 {
		t.Errorf("GRPCStatus details = %v, want 1", gs.Details())
	}
	// The reflection-based FromGRPCStatus must round-trip our own error.
	got := FromGRPCStatus(rpcErr)
	if Code(got) != codes.CodeNotFound {
		t.Errorf("FromGRPCStatus(self) code = %v, want not_found", Code(got))
	}
	if rpcGot, _ := FromError(got); len(rpcGot.Details()) != 1 {
		t.Errorf("FromGRPCStatus(self) details lost")
	}
}

func TestErrorUnwrapStillWorksWithDetails(t *testing.T) {
	base := errors.New("boom")
	rpcErr := NewError(codes.CodeInternal, base).WithDetails()
	if !errors.Is(rpcErr, base) {
		t.Error("errors.Is should find the wrapped error")
	}
}
