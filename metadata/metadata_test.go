package metadata

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestHeaderPairs(t *testing.T) {
	ctx := WithRequestMeta(context.Background(), RequestMeta{
		Header: http.Header{
			HeaderAuthorization: {"Bearer token"},
			HeaderContentType:   {ContentTypeJSON},
			"X-Custom":          {"a", "b"},
		},
	})
	pairs := HeaderPairs(ctx)
	if pairs == nil {
		t.Fatal("HeaderPairs = nil, want pairs")
	}
	// Map iteration order is random; assert membership, not order.
	got := make(map[string]string, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		got[pairs[i]] = pairs[i+1]
	}
	// Keys are lowercased: gRPC metadata keys are lowercase.
	if got["authorization"] != "Bearer token" {
		t.Errorf("authorization = %q, want Bearer token", got["authorization"])
	}
	if got["x-custom"] != "b" {
		t.Errorf("x-custom = %q, want b (last value wins in map view)", got["x-custom"])
	}
	if _, ok := got["content-type"]; ok {
		t.Error("content-type is hop-by-hop and must be skipped")
	}
}

func TestHeaderPairsEmpty(t *testing.T) {
	ctx := WithRequestMeta(context.Background(), RequestMeta{
		Header: http.Header{HeaderContentType: {ContentTypeJSON}},
	})
	if pairs := HeaderPairs(ctx); pairs != nil {
		t.Fatalf("HeaderPairs = %v, want nil (only hop-by-hop headers)", pairs)
	}
	if pairs := HeaderPairs(context.Background()); pairs != nil {
		t.Fatalf("HeaderPairs(no meta) = %v, want nil", pairs)
	}
}

func TestHeaderPairsLowercasesKeys(t *testing.T) {
	// net/http canonicalizes incoming header names to title case, but gRPC
	// metadata keys must be lowercase; the hop-by-hop check must also be
	// case-insensitive.
	ctx := WithRequestMeta(context.Background(), RequestMeta{
		Header: http.Header{
			HeaderAuthorization: {"Bearer token"},
			HeaderContentType:   {ContentTypeJSON},
		},
	})
	pairs := HeaderPairs(ctx)
	if len(pairs) != 2 || pairs[0] != "authorization" || pairs[1] != "Bearer token" {
		t.Fatalf("HeaderPairs = %v, want [authorization Bearer token]", pairs)
	}
}

func TestFormatGrpcTimeout(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0n"},
		{-time.Second, "0n"},
		{time.Nanosecond, "1n"},
		// grpc-go's EncodeDuration picks the smallest unit that fits in 8
		// digits (maxTimeoutValue = 99,999,999): nanoseconds fit up to
		// ~99ms, microseconds up to ~99s, milliseconds up to ~27h.
		{500 * time.Microsecond, "500000n"},
		{time.Millisecond, "1000000n"},
		{500 * time.Millisecond, "500000u"},
		{time.Second, "1000000u"},
		{10 * time.Second, "10000000u"},
		{90 * time.Second, "90000000u"},
		{time.Minute, "60000000u"},
		{2 * time.Hour, "7200000m"},
		// Rounding up: 1.5s cannot be represented exactly in S, so it
		// becomes 1,500,000u (grpc-go's EncodeDuration behavior).
		{1500 * time.Millisecond, "1500000u"},
	}
	for _, c := range cases {
		if got := FormatGrpcTimeout(c.in); got != c.want {
			t.Errorf("FormatGrpcTimeout(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseGrpcTimeout(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"0n", 0, true},
		{"1n", time.Nanosecond, true},
		{"500u", 500 * time.Microsecond, true},
		{"1m", time.Millisecond, true},
		{"500m", 500 * time.Millisecond, true},
		{"1S", time.Second, true},
		{"10S", 10 * time.Second, true},
		{"1M", time.Minute, true},
		{"2H", 2 * time.Hour, true},
		{"99999999S", 99999999 * time.Second, true},
		// Malformed: empty, no unit, bad unit, non-numeric, too long.
		{"", 0, false},
		{"10", 0, false},
		{"10X", 0, false},
		{"abcS", 0, false},
		{"123456789S", 0, false}, // 9 digits exceeds the 8-digit cap
		{"S", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseGrpcTimeout(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseGrpcTimeout(%q) = (%v, %v), want (%v, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseGrpcTimeoutRoundTrip(t *testing.T) {
	for _, d := range []time.Duration{
		time.Nanosecond,
		500 * time.Microsecond,
		time.Millisecond,
		500 * time.Millisecond,
		time.Second,
		10 * time.Second,
		90 * time.Second,
		time.Minute,
		2 * time.Hour,
	} {
		got, ok := ParseGrpcTimeout(FormatGrpcTimeout(d))
		if !ok {
			t.Errorf("round trip %v: parse failed", d)
			continue
		}
		// Rounding up may add up to one unit; assert the encoded value is
		// within one unit of the original.
		if got < d || got-d > time.Second {
			t.Errorf("round trip %v = %v (off by %v)", d, got, got-d)
		}
	}
}

func TestParseGrpcDeadline(t *testing.T) {
	// RFC3339.
	ts := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	if got, ok := ParseGrpcDeadline("2030-01-02T03:04:05Z"); !ok || !got.Equal(ts) {
		t.Errorf("ParseGrpcDeadline(RFC3339) = (%v, %v), want (%v, true)", got, ok, ts)
	}
	// Unix seconds.
	if got, ok := ParseGrpcDeadline(strconv.FormatInt(ts.Unix(), 10)); !ok || !got.Equal(ts) {
		t.Errorf("ParseGrpcDeadline(unix) = (%v, %v), want (%v, true)", got, ok, ts)
	}
	// Unix nanoseconds (11+ digits).
	if got, ok := ParseGrpcDeadline(strconv.FormatInt(ts.UnixNano(), 10)); !ok || !got.Equal(ts) {
		t.Errorf("ParseGrpcDeadline(unixnano) = (%v, %v), want (%v, true)", got, ok, ts)
	}
	// Malformed.
	for _, in := range []string{"", "not-a-time", "2030-13-45T99:99:99Z"} {
		if _, ok := ParseGrpcDeadline(in); ok {
			t.Errorf("ParseGrpcDeadline(%q) ok = true, want false", in)
		}
	}
}

func TestHeaderPairsSkipsDeadlineHeaders(t *testing.T) {
	// grpc-timeout/grpc-deadline are hop-by-hop: the gateway forwards the
	// deadline through the context, not as metadata.
	ctx := WithRequestMeta(t.Context(), RequestMeta{
		Header: http.Header{
			HeaderGrpcTimeout:   {"10S"},
			HeaderGrpcDeadline:  {"2030-01-02T03:04:05Z"},
			HeaderAuthorization: {"Bearer token"},
		},
	})
	pairs := HeaderPairs(ctx)
	if len(pairs) != 2 || pairs[0] != "authorization" {
		t.Fatalf("HeaderPairs = %v, want [authorization Bearer token]", pairs)
	}
}
