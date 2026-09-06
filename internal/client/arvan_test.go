package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New("test-key", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestCreateTXTRecordSendsApikeyHeader(t *testing.T) {
	var gotAuth string
	var gotBody createRecordRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(listRecordsResponse{})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "rec-123"}})
	})

	id, err := c.CreateTXTRecord(context.Background(), "example.com", "_acme-challenge.www", "token", 120)
	if err != nil {
		t.Fatalf("CreateTXTRecord: %v", err)
	}
	if id != "rec-123" {
		t.Fatalf("id = %q, want rec-123", id)
	}
	if gotAuth != "Apikey test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Apikey test-key")
	}
	if gotBody.Name != "_acme-challenge.www" || gotBody.Value.Text != "token" || gotBody.TTL != 120 {
		t.Fatalf("unexpected body: %+v", gotBody)
	}
}

func TestCreateTXTRecordIsIdempotent(t *testing.T) {
	posts := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(listRecordsResponse{
				Data: []Record{{ID: "existing", Type: "txt", Name: "_acme-challenge", Value: RecordValue{Text: "token"}}},
			})
			return
		}
		posts++
		w.WriteHeader(http.StatusCreated)
	})

	id, err := c.CreateTXTRecord(context.Background(), "example.com", "_acme-challenge.example.com", "token", 120)
	if err != nil {
		t.Fatalf("CreateTXTRecord: %v", err)
	}
	if id != "existing" || posts != 0 {
		t.Fatalf("expected reuse of existing record, got id=%q posts=%d", id, posts)
	}
}

func TestDeleteRecordNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if err := c.DeleteRecord(context.Background(), "example.com", "rec-1"); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("err = %v, want ErrRecordNotFound", err)
	}
}

func TestDoRetriesOnServerError(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(listRecordsResponse{})
	})
	if _, err := c.FindTXTRecords(context.Background(), "example.com", "_acme-challenge"); err != nil {
		t.Fatalf("FindTXTRecords: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestFindTXTRecordsPaginates(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		resp := listRecordsResponse{}
		resp.Meta.LastPage = 2
		switch page {
		case "1":
			resp.Meta.CurrentPage = 1
			resp.Data = []Record{{ID: "a", Type: "txt", Name: "other"}}
		case "2":
			resp.Meta.CurrentPage = 2
			resp.Data = []Record{{ID: "b", Type: "txt", Name: "_acme-challenge"}}
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	recs, err := c.FindTXTRecords(context.Background(), "example.com", "_acme-challenge")
	if err != nil {
		t.Fatalf("FindTXTRecords: %v", err)
	}
	if len(recs) != 1 || recs[0].ID != "b" {
		t.Fatalf("unexpected records: %+v", recs)
	}
}

func TestNewRejectsEmptyKey(t *testing.T) {
	if _, err := New("  "); err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestNormaliseName(t *testing.T) {
	cases := []struct{ name, domain, want string }{
		{"_acme-challenge.www.example.com.", "example.com.", "_acme-challenge.www"},
		{"_acme-challenge.example.com", "example.com", "_acme-challenge"},
		{"example.com", "example.com", "@"},
		{"_acme-challenge.apps.cluster.example.com", "example.com", "_acme-challenge.apps.cluster"},
	}
	for _, tc := range cases {
		if got := normaliseName(tc.name, tc.domain); got != tc.want {
			t.Errorf("normaliseName(%q,%q) = %q, want %q", tc.name, tc.domain, got, tc.want)
		}
	}
}

func TestAPIErrorTextIncludesMessage(t *testing.T) {
	body, _ := json.Marshal(apiError{Message: "invalid", Errors: map[string][]string{"value": {"required"}}})
	got := apiErrorText(body, 422)
	if !strings.Contains(got, "invalid") || !strings.Contains(got, "value: required") {
		t.Fatalf("apiErrorText = %q", got)
	}
}
