package codes

import "net/http"

// Code represents an RPC error code, compatible with gRPC status codes.
type Code int

const (
	CodeOK                 Code = 0
	CodeCanceled           Code = 1
	CodeUnknown            Code = 2
	CodeInvalidArgument    Code = 3
	CodeDeadlineExceeded   Code = 4
	CodeNotFound           Code = 5
	CodeAlreadyExists      Code = 6
	CodePermissionDenied   Code = 7
	CodeResourceExhausted  Code = 8
	CodeFailedPrecondition Code = 9
	CodeAborted            Code = 10
	CodeOutOfRange         Code = 11
	CodeUnimplemented      Code = 12
	CodeInternal           Code = 13
	CodeUnavailable        Code = 14
	CodeDataLoss           Code = 15
	CodeUnauthenticated    Code = 16
)

// String returns the human-readable name for a code.
func (c Code) String() string {
	switch c {
	case CodeOK:
		return "ok"
	case CodeCanceled:
		return "canceled"
	case CodeUnknown:
		return "unknown"
	case CodeInvalidArgument:
		return "invalid_argument"
	case CodeDeadlineExceeded:
		return "deadline_exceeded"
	case CodeNotFound:
		return "not_found"
	case CodeAlreadyExists:
		return "already_exists"
	case CodePermissionDenied:
		return "permission_denied"
	case CodeResourceExhausted:
		return "resource_exhausted"
	case CodeFailedPrecondition:
		return "failed_precondition"
	case CodeAborted:
		return "aborted"
	case CodeOutOfRange:
		return "out_of_range"
	case CodeUnimplemented:
		return "unimplemented"
	case CodeInternal:
		return "internal"
	case CodeUnavailable:
		return "unavailable"
	case CodeDataLoss:
		return "data_loss"
	case CodeUnauthenticated:
		return "unauthenticated"
	default:
		return "unknown"
	}
}

// HTTPStatus returns the HTTP status code mapped to this RPC code.
func (c Code) HTTPStatus() int {
	switch c {
	case CodeOK:
		return http.StatusOK
	case CodeCanceled:
		return 499 // Client Closed Request
	case CodeUnknown:
		return http.StatusInternalServerError
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeDeadlineExceeded:
		return http.StatusGatewayTimeout
	case CodeNotFound:
		return http.StatusNotFound
	case CodeAlreadyExists:
		return http.StatusConflict
	case CodePermissionDenied:
		return http.StatusForbidden
	case CodeResourceExhausted:
		return http.StatusTooManyRequests
	case CodeFailedPrecondition:
		return http.StatusBadRequest
	case CodeAborted:
		return http.StatusConflict
	case CodeOutOfRange:
		return http.StatusBadRequest
	case CodeUnimplemented:
		return http.StatusNotImplemented
	case CodeInternal:
		return http.StatusInternalServerError
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	case CodeDataLoss:
		return http.StatusInternalServerError
	case CodeUnauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

// codeNames is the reverse lookup used when parsing error responses.
var codeNames = func() map[string]Code {
	m := make(map[string]Code, 17)
	for c := CodeOK; c <= CodeUnauthenticated; c++ {
		m[c.String()] = c
	}
	return m
}()

// CodeFromString parses a code name like "invalid_argument".
func CodeFromString(s string) (Code, bool) {
	c, ok := codeNames[s]
	return c, ok
}

// CodeFromHTTPStatus maps an HTTP status code to an RPC code, the inverse
// of Code.HTTPStatus. It is used by observability hooks to report the RPC
// code for a completed request. Unknown statuses map to CodeUnknown.
func CodeFromHTTPStatus(status int) Code {
	switch status {
	case http.StatusOK:
		return CodeOK
	case 499:
		return CodeCanceled
	case http.StatusBadRequest:
		return CodeInvalidArgument
	case http.StatusGatewayTimeout:
		return CodeDeadlineExceeded
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusConflict:
		return CodeAborted
	case http.StatusForbidden:
		return CodePermissionDenied
	case http.StatusTooManyRequests:
		return CodeResourceExhausted
	case http.StatusNotImplemented:
		return CodeUnimplemented
	case http.StatusInternalServerError:
		return CodeInternal
	case http.StatusServiceUnavailable:
		return CodeUnavailable
	case http.StatusUnauthorized:
		return CodeUnauthenticated
	default:
		return CodeUnknown
	}
}