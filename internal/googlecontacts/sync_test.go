package googlecontacts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/settings"

	_ "github.com/mattn/go-sqlite3"
)

func newTestSyncer(t *testing.T, handler http.HandlerFunc) (*Syncer, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	// enttest already creates schema
	cfg := &config.Config{GoogleContactsEnabled: true, GoogleDeletePolicy: "keep"}
	store := settings.New(client)
	// seed app_config
	if err := store.EnsureSeeded(context.Background()); err != nil {
		t.Fatal(err)
	}
	gc := NewWithBaseURL(&StaticTokenTransport{Token: "t"}, srv.URL)
	syncer := NewSyncer(cfg, client, store, gc)
	// store client for cleanup
	t.Cleanup(func() {
		srv.Close()
		client.Close()
	})
	return syncer, srv
}

func TestEngineCreateAndUpdate(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"connections": []map[string]any{
				{"resourceName": "people/1", "names": []map[string]any{{"displayName": "Alice"}}, "birthdays": []map[string]any{{"date": map[string]any{"year": 1990, "month": 5, "day": 10}}}, "biographies": []map[string]any{{"value": "hello"}}},
			},
			"nextSyncToken": "tok1",
		}
		json.NewEncoder(w).Encode(resp)
	}
	syncer, _ := newTestSyncer(t, handler)
	ctx := context.Background()
	res, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 {
		t.Fatalf("created %d", res.Created)
	}
	// second sync with same resource but updated name
	handler2 := func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"connections": []map[string]any{
				{"resourceName": "people/1", "names": []map[string]any{{"displayName": "Alice Updated"}}, "birthdays": []map[string]any{{"date": map[string]any{"year": 1990, "month": 5, "day": 10}}}},
			},
			"nextSyncToken": "tok2",
		}
		json.NewEncoder(w).Encode(resp)
	}
	srv2 := httptest.NewServer(http.HandlerFunc(handler2))
	defer srv2.Close()
	syncer.gc = NewWithBaseURL(&StaticTokenTransport{Token: "t"}, srv2.URL)
	// reset sync token to force full fetch
	syncer.cfg.GoogleDeletePolicy = "keep"
	_ = syncer.settings.SetGoogleSyncToken(ctx, "")
	res, err = syncer.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated %d", res.Updated)
	}
}

func TestFallbackDedup(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	cfg := &config.Config{GoogleContactsEnabled: true, GoogleDeletePolicy: "keep"}
	store := settings.New(client)
	if err := store.EnsureSeeded(ctx); err != nil {
		t.Fatal(err)
	}
	people := repository.NewPersonRepository(client)
	events := repository.NewEventRepository(client)
	p, err := people.Create(ctx, "Bob", "", "")
	if err != nil {
		t.Fatal(err)
	}
	bday := time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, p.ID, "birthday", bday, "Birthday of Bob"); err != nil {
		t.Fatal(err)
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"connections": []map[string]any{
				{"resourceName": "people/999", "names": []map[string]any{{"displayName": "Bob"}}, "birthdays": []map[string]any{{"date": map[string]any{"year": 1985, "month": 3, "day": 15}}}},
			},
			"nextSyncToken": "tok",
		}
		json.NewEncoder(w).Encode(resp)
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()
	gc := NewWithBaseURL(&StaticTokenTransport{Token: "t"}, srv.URL)
	syncer := NewSyncer(cfg, client, store, gc)
	res, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 || res.Updated != 1 {
		t.Fatalf("expected fallback link (updated=1, created=0), got created=%d updated=%d", res.Created, res.Updated)
	}
	linked, err := people.FindByGoogleResourceName(ctx, "people/999")
	if err != nil {
		t.Fatalf("resource name not linked: %v", err)
	}
	if linked.ID != p.ID {
		t.Fatalf("linked wrong person %d, want %d", linked.ID, p.ID)
	}
}

// TestIncrementalDeltaDoesNotSweep guards against treating an incremental
// (syncToken) response as a full roster: contacts absent from a delta were
// not necessarily deleted remotely and must keep their links.
func TestIncrementalDeltaDoesNotSweep(t *testing.T) {
	full := func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"connections": []map[string]any{
				{"resourceName": "people/1", "names": []map[string]any{{"displayName": "Alice"}}},
				{"resourceName": "people/2", "names": []map[string]any{{"displayName": "Bob"}}},
			},
			"nextSyncToken": "tok1",
		}
		json.NewEncoder(w).Encode(resp)
	}
	syncer, _ := newTestSyncer(t, full)
	ctx := context.Background()
	if res, err := syncer.Sync(ctx); err != nil || res.Created != 2 {
		t.Fatalf("full sync: created=%d err=%v", res.Created, err)
	}
	delta := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("syncToken") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		resp := map[string]any{
			"connections": []map[string]any{
				{"resourceName": "people/1", "names": []map[string]any{{"displayName": "Alice Updated"}}},
			},
			"nextSyncToken": "tok2",
		}
		json.NewEncoder(w).Encode(resp)
	}
	srv2 := httptest.NewServer(http.HandlerFunc(delta))
	defer srv2.Close()
	syncer.gc = NewWithBaseURL(&StaticTokenTransport{Token: "t"}, srv2.URL)
	res, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 || res.Deleted != 0 {
		t.Fatalf("delta sync: updated=%d deleted=%d, want updated=1 deleted=0", res.Updated, res.Deleted)
	}
	people := repository.NewPersonRepository(syncer.client)
	if _, err := people.FindByGoogleResourceName(ctx, "people/2"); err != nil {
		t.Fatalf("Bob was falsely unlinked by delta sweep: %v", err)
	}
}

// TestDeltaTombstoneAppliesPolicy verifies metadata.deleted tombstones in
// incremental responses follow the deletion policy.
func TestDeltaTombstoneAppliesPolicy(t *testing.T) {
	t.Run("keep unlinks", func(t *testing.T) {
		full := func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{
				"connections":   []map[string]any{{"resourceName": "people/1", "names": []map[string]any{{"displayName": "Gone"}}}},
				"nextSyncToken": "tok1",
			})
		}
		syncer, _ := newTestSyncer(t, full)
		ctx := context.Background()
		if _, err := syncer.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		delta := func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{
				"connections":   []map[string]any{{"resourceName": "people/1", "metadata": map[string]any{"deleted": true}}},
				"nextSyncToken": "tok2",
			})
		}
		srv2 := httptest.NewServer(http.HandlerFunc(delta))
		defer srv2.Close()
		syncer.gc = NewWithBaseURL(&StaticTokenTransport{Token: "t"}, srv2.URL)
		res, err := syncer.Sync(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.Deleted != 1 {
			t.Fatalf("deleted=%d, want 1", res.Deleted)
		}
		people := repository.NewPersonRepository(syncer.client)
		p, err := people.FindByGoogleResourceName(ctx, "people/1")
		if err == nil && p != nil {
			t.Fatal("link should be cleared under keep policy")
		}
	})
	t.Run("delete removes person", func(t *testing.T) {
		full := func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{
				"connections":   []map[string]any{{"resourceName": "people/1", "names": []map[string]any{{"displayName": "Gone"}}}},
				"nextSyncToken": "tok1",
			})
		}
		syncer, _ := newTestSyncer(t, full)
		syncer.cfg.GoogleDeletePolicy = "delete"
		ctx := context.Background()
		if _, err := syncer.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		delta := func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{
				"connections":   []map[string]any{{"resourceName": "people/1", "metadata": map[string]any{"deleted": true}}},
				"nextSyncToken": "tok2",
			})
		}
		srv2 := httptest.NewServer(http.HandlerFunc(delta))
		defer srv2.Close()
		syncer.gc = NewWithBaseURL(&StaticTokenTransport{Token: "t"}, srv2.URL)
		if _, err := syncer.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		if n := syncer.client.Person.Query().CountX(ctx); n != 0 {
			t.Fatalf("person count=%d, want 0 under delete policy", n)
		}
	})
}

func TestBirthdayLessSkipped(t *testing.T) {
	// Spec says still create without birthday, so test that it creates
	handler := func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"connections": []map[string]any{
				{"resourceName": "people/1", "names": []map[string]any{{"displayName": "NoBirthday"}}},
			},
			"nextSyncToken": "tok",
		}
		json.NewEncoder(w).Encode(resp)
	}
	syncer, _ := newTestSyncer(t, handler)
	res, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 {
		t.Fatalf("expected create for birthday-less, got %d", res.Created)
	}
}
