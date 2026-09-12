package runtime

import (
	"net/url"
	"testing"
	"time"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/status"
)

type populateNested struct {
	Note string `json:"note"`
}

type populateMsg struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Age     int             `json:"age"`
	Score   float64         `json:"score"`
	Active  bool            `json:"active"`
	Tags    []string        `json:"tags"`
	Status  *populateNested `json:"status"`
	Created time.Time       `json:"created"`
	Ignored string          `json:"-"`
	NoTag   string
}

func TestPopulateMessage(t *testing.T) {
	msg := &populateMsg{}
	params := map[string]string{
		"id":          "42",
		"status.note": "hello",
	}
	query := url.Values{
		"name":    {"Zara"},
		"age":     {"30"},
		"score":   {"9.5"},
		"active":  {"true"},
		"tags":    {"a", "b", "c"},
		"created": {"2026-01-02T15:04:05Z"},
		"unknown": {"ignored"},
	}

	err := PopulateMessage(msg, params, query, []byte{}, "", encoding.JSONCodec{})
	if err != nil {
		t.Fatalf("PopulateMessage: %v", err)
	}

	if msg.ID != "42" {
		t.Errorf("ID = %q, want 42", msg.ID)
	}
	if msg.Name != "Zara" {
		t.Errorf("Name = %q, want Zara", msg.Name)
	}
	if msg.Age != 30 {
		t.Errorf("Age = %d, want 30", msg.Age)
	}
	if msg.Score != 9.5 {
		t.Errorf("Score = %v, want 9.5", msg.Score)
	}
	if !msg.Active {
		t.Error("Active = false, want true")
	}
	if len(msg.Tags) != 3 || msg.Tags[0] != "a" || msg.Tags[2] != "c" {
		t.Errorf("Tags = %v, want [a b c]", msg.Tags)
	}
	if msg.Status == nil || msg.Status.Note != "hello" {
		t.Errorf("Status = %+v, want note=hello", msg.Status)
	}
	wantTime := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	if !msg.Created.Equal(wantTime) {
		t.Errorf("Created = %v, want %v", msg.Created, wantTime)
	}
}

func TestPopulateMessagePathError(t *testing.T) {
	msg := &populateMsg{}
	err := PopulateMessage(msg, map[string]string{"nope": "x"}, nil, []byte{}, "", encoding.JSONCodec{})
	if err == nil {
		t.Fatal("expected error for unknown path param")
	}
	if status.Code(err) != codes.CodeInvalidArgument {
		t.Errorf("status.Code(err) = %v, want %v", status.Code(err), codes.CodeInvalidArgument)
	}
}

func TestPopulateMessageBodyStar(t *testing.T) {
	msg := &populateMsg{}
	body := `{"id":"1","name":"from-body","age":25}`
	err := PopulateMessage(msg, map[string]string{}, nil, []byte(body), "*", encoding.JSONCodec{})
	if err != nil {
		t.Fatalf("PopulateMessage: %v", err)
	}
	if msg.ID != "1" || msg.Name != "from-body" || msg.Age != 25 {
		t.Errorf("msg = %+v, want body fields populated", msg)
	}
}

func TestPopulateMessageBodyField(t *testing.T) {
	msg := &populateMsg{}
	body := `{"note":"nested-from-body"}`
	err := PopulateMessage(msg, map[string]string{"id": "7"}, nil, []byte(body), "status", encoding.JSONCodec{})
	if err != nil {
		t.Fatalf("PopulateMessage: %v", err)
	}
	if msg.ID != "7" {
		t.Errorf("ID = %q, want 7", msg.ID)
	}
	if msg.Status == nil || msg.Status.Note != "nested-from-body" {
		t.Errorf("Status = %+v, want note=nested-from-body", msg.Status)
	}
}

func TestPopulateMessageBodyStarSkipsQuery(t *testing.T) {
	msg := &populateMsg{}
	body := `{"id":"1"}`
	query := url.Values{"name": {"should-not-apply"}}
	err := PopulateMessage(msg, map[string]string{}, query, []byte(body), "*", encoding.JSONCodec{})
	if err != nil {
		t.Fatalf("PopulateMessage: %v", err)
	}
	if msg.Name != "" {
		t.Errorf("Name = %q, want empty (query skipped with body:*)", msg.Name)
	}
}

func TestPopulateMessageInvalidInt(t *testing.T) {
	msg := &populateMsg{}
	err := PopulateMessage(msg, map[string]string{}, url.Values{"age": {"not-a-number"}}, []byte{}, "", encoding.JSONCodec{})
	if err == nil {
		t.Fatal("expected error for invalid int")
	}
	if status.Code(err) != codes.CodeInvalidArgument {
		t.Errorf("status.Code(err) = %v, want %v", status.Code(err), codes.CodeInvalidArgument)
	}
}

func TestPopulateMessageBodyFieldUnknown(t *testing.T) {
	msg := &populateMsg{}
	err := PopulateMessage(msg, nil, nil, []byte(`{}`), "nope", encoding.JSONCodec{})
	if err == nil {
		t.Fatal("expected error for unknown body field")
	}
}

// TestPopulateQuery pins the automatic query-binding contract used by the
// generated request builders: dotted paths resolve nested messages, repeated
// params fill repeated fields, unknown params are ignored, and names in
// bound (path/body fields) win over the query.
func TestPopulateQuery(t *testing.T) {
	msg := &populateMsg{}
	query := url.Values{
		"name":        {"Zara"},
		"tags":        {"a", "b"},
		"status.note": {"nested"},
		"unknown":     {"ignored"},
		"id":          {"99"}, // bound: path/body wins
	}
	err := PopulateQuery(msg, query, "id")
	if err != nil {
		t.Fatalf("PopulateQuery: %v", err)
	}
	if msg.Name != "Zara" {
		t.Errorf("Name = %q, want Zara", msg.Name)
	}
	if len(msg.Tags) != 2 || msg.Tags[0] != "a" || msg.Tags[1] != "b" {
		t.Errorf("Tags = %v, want [a b]", msg.Tags)
	}
	if msg.Status == nil || msg.Status.Note != "nested" {
		t.Errorf("Status = %+v, want note=nested", msg.Status)
	}
	if msg.ID != "" {
		t.Errorf("ID = %q, want empty (bound name skipped)", msg.ID)
	}
}

// TestPopulateQueryInvalid pins the error path: a query param that matches a
// field but fails to parse returns an InvalidArgument error.
func TestPopulateQueryInvalid(t *testing.T) {
	msg := &populateMsg{}
	err := PopulateQuery(msg, url.Values{"age": {"not-a-number"}})
	if err == nil {
		t.Fatal("expected error for invalid int")
	}
	if status.Code(err) != codes.CodeInvalidArgument {
		t.Errorf("status.Code(err) = %v, want %v", status.Code(err), codes.CodeInvalidArgument)
	}
}
