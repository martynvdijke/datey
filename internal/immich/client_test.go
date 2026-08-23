package immich

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	if got := NormalizeName("  José O'Connor  "); got != "joséoconnor" {
		t.Fatalf("got %q", got)
	}
}

func TestPeoplePaginatedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"people":[{"id":"1","name":"Jane Doe"},{"id":"2","name":"John"}],
			"total":2,"hidden":0,"hasNextPage":false}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "key")
	people, err := c.People(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(people) != 2 || people[0].Name != "Jane Doe" {
		t.Fatalf("unexpected people: %#v", people)
	}
}

func TestPeopleBareArrayResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"1","name":"Jane Doe"},{"id":"2","name":"John"}]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "key")
	people, err := c.People(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(people) != 2 || people[1].Name != "John" {
		t.Fatalf("unexpected people: %#v", people)
	}
}

func TestPeopleNon2xxReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(srv.URL, "key")
	if _, err := c.People(context.Background()); err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestEnabled(t *testing.T) {
	if New("", "key").Enabled() {
		t.Fatal("empty baseURL should be disabled")
	}
	if New("https://immich.example", "").Enabled() {
		t.Fatal("empty apiKey should be disabled")
	}
	if !New("https://immich.example", "key").Enabled() {
		t.Fatal("configured client should be enabled")
	}
}

func TestRequestSetsAPIKeyHeader(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"people":[]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "secret")
	if _, err := c.People(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKey != "secret" {
		t.Fatalf("expected x-api-key header %q, got %q", "secret", gotKey)
	}
}

func TestThumbnailSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/people/abc/thumbnail" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("imgbytes"))
	}))
	defer srv.Close()

	c := New(srv.URL, "key")
	rc, ct, err := c.Thumbnail(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()
	if ct != "image/jpeg" {
		t.Fatalf("expected content type image/jpeg, got %q", ct)
	}
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if string(body) != "imgbytes" {
		t.Fatalf("unexpected body %q", string(body))
	}
}

func TestThumbnailNon2xxReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, "key")
	if _, _, err := c.Thumbnail(context.Background(), "abc"); err == nil {
		t.Fatal("expected error for non-2xx thumbnail response")
	}
}

func TestExactMatch(t *testing.T) {
	people := []Person{
		{ID: "1", Name: "Jane Doe"},
		{ID: "2", Name: "John Smith"},
	}
	if got := ExactMatch("Jane Doe", people); got == nil || got.ID != "1" {
		t.Fatalf("expected match to Jane Doe, got %#v", got)
	}
	if got := ExactMatch("Nobody", people); got != nil {
		t.Fatalf("expected no match, got %#v", got)
	}
	// Ambiguous: two people normalize to the same string.
	ambiguous := []Person{{ID: "1", Name: "Jane Doe"}, {ID: "2", Name: "Jane-Doe"}}
	if got := ExactMatch("Jane Doe", ambiguous); got != nil {
		t.Fatalf("expected ambiguous match to be rejected, got %#v", got)
	}
}

func TestExactMatchRejectsAmbiguousNames(t *testing.T) {
	people := []Person{{ID: "1", Name: "Jane Doe"}, {ID: "2", Name: "Jane-Doe"}}
	if got := ExactMatch("Jane Doe", people); got != nil {
		t.Fatalf("expected ambiguous match to be rejected: %#v", got)
	}
}
