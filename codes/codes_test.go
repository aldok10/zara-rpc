package codes

import (
	"net/http"
	"testing"
)

func TestCodeString(t *testing.T) {
	tests := []struct {
		code Code
		want string
	}{
		{CodeOK, "ok"},
		{CodeNotFound, "not_found"},
		{CodeUnauthenticated, "unauthenticated"},
		{Code(999), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.code.String(); got != tt.want {
			t.Errorf("Code(%d).String() = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestCodeHTTPStatus(t *testing.T) {
	tests := []struct {
		code Code
		want int
	}{
		{CodeOK, http.StatusOK},
		{CodeInvalidArgument, http.StatusBadRequest},
		{CodeNotFound, http.StatusNotFound},
		{CodeAlreadyExists, http.StatusConflict},
		{CodeUnauthenticated, http.StatusUnauthorized},
		{CodePermissionDenied, http.StatusForbidden},
		{CodeUnimplemented, http.StatusNotImplemented},
		{CodeUnavailable, http.StatusServiceUnavailable},
		{CodeDeadlineExceeded, http.StatusGatewayTimeout},
	}
	for _, tt := range tests {
		if got := tt.code.HTTPStatus(); got != tt.want {
			t.Errorf("Code(%v).HTTPStatus() = %d, want %d", tt.code, got, tt.want)
		}
	}
}