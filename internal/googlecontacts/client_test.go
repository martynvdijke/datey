package googlecontacts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListPagination(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/people/me/connections", func(w http.ResponseWriter, r *http.Request) {
		pageToken := r.URL.Query().Get("pageToken")
		var resp map[string]any
		if pageToken == "" {
			resp = map[string]any{
				"connections":   []map[string]any{{"resourceName": "people/1", "names": []map[string]any{{"displayName": "Alice"}}}},
				"nextPageToken": "tok2",
			}
		} else {
			resp = map[string]any{
				"connections":   []map[string]any{{"resourceName": "people/2", "names": []map[string]any{{"displayName": "Bob"}}}},
				"nextSyncToken": "sync123",
			}
		}
		json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewWithBaseURL(&StaticTokenTransport{Token: "mytoken"}, srv.URL)
	res, err := client.ListContacts(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.Contacts) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(res.Contacts))
	}
	if res.NextSyncToken != "sync123" {
		t.Fatalf("sync token %q", res.NextSyncToken)
	}
}

func TestSyncTokenReuse(t *testing.T) {
	var gotSyncToken string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/people/me/connections", func(w http.ResponseWriter, r *http.Request) {
		gotSyncToken = r.URL.Query().Get("syncToken")
		json.NewEncoder(w).Encode(map[string]any{"connections": []any{}, "nextSyncToken": "next"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := NewWithBaseURL(&StaticTokenTransport{Token: "t"}, srv.URL)
	_, err := client.ListContacts(context.Background(), "mySync")
	if err != nil {
		t.Fatal(err)
	}
	if gotSyncToken != "mySync" {
		t.Fatalf("expected syncToken mySync got %q", gotSyncToken)
	}
}

func TestAuthHeaders(t *testing.T) {
	var auth string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/people/me/connections", func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"connections": []any{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := NewWithBaseURL(&StaticTokenTransport{Token: "secret123"}, srv.URL)
	_, err := client.ListContacts(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer secret123" {
		t.Fatalf("auth header %q", auth)
	}
}

func TestBirthdayParsing(t *testing.T) {
	c := Contact{ResourceName: "people/1", Birthdays: []Birthday{{Date: &Date{Year: 1990, Month: 5, Day: 10}}}}
	if bt := c.BirthdayTime(); bt == nil || bt.Month() != 5 || bt.Day() != 10 {
		t.Fatal("birthday date parse failed")
	}
	c2 := Contact{ResourceName: "people/2", Birthdays: []Birthday{{Text: "1995-11-20"}}}
	if bt := c2.BirthdayTime(); bt == nil {
		t.Fatal("birthday text parse failed")
	}
	_ = c.BiographyText()
}
