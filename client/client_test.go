package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/metadata"
	"github.com/aldok10/zara-rpc/status"
)

type testUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type testGetReq struct {
	ID string `json:"id"`
}

type testCreateReq struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type testListReq struct {
	PageSize int `json:"page_size"`
}

type testListResp struct {
	Users []*testUser `json:"users"`
}

func TestClientUnaryRoundTrip(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/users/42", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{"id":"42","name":"Zara","email":"zara@example.com"}`))
	})
	mux.HandleFunc("/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{"id":"new","name":"Budi","email":"budi@example.com"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClientBase(srv.URL)

	// GET /v1/users/{id} -- path param, no body.
	var got testUser
	if err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users/{id}", "", &testGetReq{ID: "42"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "42" || got.Name != "Zara" {
		t.Errorf("got = %+v, want id=42 name=Zara", got)
	}

	// POST /v1/users -- body: "*".
	var created testUser
	if err := c.DoUnary(context.Background(), http.MethodPost, "/v1/users", "*", &testCreateReq{Name: "Budi", Email: "budi@example.com"}, &created); err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Name != "Budi" || created.Email != "budi@example.com" {
		t.Errorf("created = %+v, want name=Budi email=budi@example.com", created)
	}
}

func TestClientErrorPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":"not_found","message":"user \"missing\" not found"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	var got testUser
	err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users/{id}", "", &testGetReq{ID: "missing"}, &got)
	if err == nil {
		t.Fatal("expected error")
	}
	if status.Code(err) != codes.CodeNotFound {
		t.Errorf("status.Code(err) = %v, want %v", status.Code(err), codes.CodeNotFound)
	}
}

func TestClientPathParamSubstitution(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{"id":"42","name":"Zara","email":"z@e.com"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	var got testUser
	if err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users/{id}", "", &testGetReq{ID: "42"}, &got); err != nil {
		t.Fatalf("call: %v", err)
	}
	if gotPath != "/v1/users/42" {
		t.Errorf("path = %q, want /v1/users/42", gotPath)
	}
}

func TestClientQueryParams(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{"users":[]}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	var got testListResp
	if err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users", "", &testListReq{PageSize: 25}, &got); err != nil {
		t.Fatalf("call: %v", err)
	}
	if gotQuery != "page_size=25" {
		t.Errorf("query = %q, want page_size=25", gotQuery)
	}
}

func TestClientBodyField(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{"id":"42","name":"Zara","email":"z@e.com"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	var got testUser
	if err := c.DoUnary(context.Background(), http.MethodPost, "/v1/users", "name", &testCreateReq{Name: "Budi", Email: "budi@example.com"}, &got); err != nil {
		t.Fatalf("call: %v", err)
	}
	if gotBody != `"Budi"` {
		t.Errorf("body = %q, want \"Budi\"", gotBody)
	}
}

// TestClientNDJSONStream verifies the client-streaming wire format: every
// message must be newline-delimited, or the server's line reader
// concatenates messages and sees EOF before any complete message.
func TestClientNDJSONStream(t *testing.T) {
	var gotLines []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc := newLineScanner(r.Body)
		for sc.Scan() {
			gotLines = append(gotLines, sc.Text())
		}
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{"count":3}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	stream, err := c.DoClientStream(context.Background(), http.MethodPost, "/v1/users:upload", reflect.TypeOf(testListResp{}), )
	if err != nil {
		t.Fatalf("DoClientStream: %v", err)
	}
	for _, name := range []string{"A", "B", "C"} {
		if err := stream.Send(&testUser{Name: name}); err != nil {
			t.Fatalf("Send(%s): %v", name, err)
		}
	}
	resp, err := stream.CloseAndReceive()
	if err != nil {
		t.Fatalf("CloseAndReceive: %v", err)
	}
	if len(gotLines) != 3 {
		t.Fatalf("server received %d lines, want 3: %q", len(gotLines), gotLines)
	}
	for i, name := range []string{"A", "B", "C"} {
		want := `{"id":"","name":"` + name + `","email":""}`
		if gotLines[i] != want {
			t.Errorf("line %d = %q, want %q", i, gotLines[i], want)
		}
	}
	if resp.(*testListResp) == nil {
		t.Errorf("resp = %v, want *testListResp", resp)
	}
}

// newLineScanner is a tiny bufio.Scanner wrapper so the test reads the
// request body exactly like the server's NDJSON reader (line-delimited).
func newLineScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	return sc
}

// BenchmarkClientDoUnary measures the full unary round trip: path
// substitution, request encode, HTTP call, response read + decode.
func BenchmarkClientDoUnary(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		w.Write([]byte(`{"id":"42","name":"Zara","email":"zara@example.com"}`))
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	req := &testGetReq{ID: "42"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var got testUser
		if err := c.DoUnary(context.Background(), http.MethodGet, "/v1/users/{id}", "", req, &got); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClientSSEServerStream(b *testing.B) {
	const events = 10
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeEventStream)
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < events; i++ {
			fmt.Fprintf(w, "data: {\"id\":\"%d\"}\n\n", i)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	req := &testGetReq{ID: "42"}
	msgTyp := reflect.TypeOf(testUser{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := c.DoServerStream(context.Background(), http.MethodGet, "/v1/stream/{id}", req, msgTyp)
		if err != nil {
			b.Fatal(err)
		}
		for j := 0; j < events; j++ {
			if _, err := stream.Receive(); err != nil {
				b.Fatalf("receive %d: %v", j, err)
			}
		}
		if err := stream.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClientNDJSONClientStream(b *testing.B) {
	const messages = 10
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc := bufio.NewScanner(r.Body)
		count := 0
		for sc.Scan() {
			count++
		}
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		fmt.Fprintf(w, `{"count":%d}`, count)
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := c.DoClientStream(context.Background(), http.MethodPost, "/v1/upload", reflect.TypeOf(testListResp{}))
		if err != nil {
			b.Fatal(err)
		}
		for j := 0; j < messages; j++ {
			if err := stream.Send(&testUser{Name: fmt.Sprintf("user%d", j)}); err != nil {
				b.Fatalf("send %d: %v", j, err)
			}
		}
		if _, err := stream.CloseAndReceive(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClientWSBidi(b *testing.B) {
	const messages = 10
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		for {
			typ, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			if err := conn.Write(r.Context(), typ, data); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := c.DoBidiStream(context.Background(), "/v1/chat", reflect.TypeOf(testUser{}))
		if err != nil {
			b.Fatal(err)
		}
		for j := 0; j < messages; j++ {
			msg := &testUser{Name: fmt.Sprintf("user%d", j)}
			if err := stream.Send(msg); err != nil {
				b.Fatalf("send %d: %v", j, err)
			}
			if _, err := stream.Receive(); err != nil {
				b.Fatalf("receive %d: %v", j, err)
			}
		}
		if err := stream.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClientSSEServerStreamLarge(b *testing.B) {
	payload := strings.Repeat("x", 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeEventStream)
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		data, _ := json.Marshal(map[string]string{"body": payload})
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	msgTyp := reflect.TypeOf(testUser{})
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := c.DoServerStream(context.Background(), http.MethodGet, "/v1/stream", nil, msgTyp)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := stream.Receive(); err != nil {
			b.Fatal(err)
		}
		stream.Close()
	}
}

func BenchmarkClientNDJSONClientStreamLarge(b *testing.B) {
	payload := strings.Repeat("x", 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc := bufio.NewScanner(r.Body)
		count := 0
		for sc.Scan() {
			count++
		}
		w.Header().Set(metadata.HeaderContentType, metadata.ContentTypeJSON)
		fmt.Fprintf(w, `{"count":%d}`, count)
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	largeMsg := &testUser{ID: "1", Name: payload, Email: "x@example.com"}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := c.DoClientStream(context.Background(), http.MethodPost, "/v1/upload", reflect.TypeOf(testListResp{}))
		if err != nil {
			b.Fatal(err)
		}
		if err := stream.Send(largeMsg); err != nil {
			b.Fatal(err)
		}
		if _, err := stream.CloseAndReceive(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClientWSBidiLarge(b *testing.B) {
	payload := strings.Repeat("x", 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		for {
			typ, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			if err := conn.Write(r.Context(), typ, data); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	c := NewClientBase(srv.URL)
	largeMsg := &testUser{ID: "1", Name: payload, Email: "x@example.com"}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := c.DoBidiStream(context.Background(), "/v1/chat", reflect.TypeOf(testUser{}))
		if err != nil {
			b.Fatal(err)
		}
		if err := stream.Send(largeMsg); err != nil {
			b.Fatal(err)
		}
		if _, err := stream.Receive(); err != nil {
			b.Fatal(err)
		}
		stream.Close()
	}
}
