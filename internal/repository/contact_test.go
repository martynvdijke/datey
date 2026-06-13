package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestContactRepo(t *testing.T) *ContactRepository {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_contact_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewContactRepository(client)
}

func seedContact(t *testing.T, repo *ContactRepository, name, notes string) int {
	t.Helper()
	c, err := repo.Create(context.Background(), name, notes)
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	return c.ID
}

func TestContactCreate(t *testing.T) {
	repo := newTestContactRepo(t)

	c, err := repo.Create(context.Background(), "Alice", "Friend")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.Name != "Alice" {
		t.Errorf("expected Name 'Alice', got %q", c.Name)
	}
	if c.Notes != "Friend" {
		t.Errorf("expected Notes 'Friend', got %q", c.Notes)
	}
	if c.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
}

func TestContactGet(t *testing.T) {
	repo := newTestContactRepo(t)
	id := seedContact(t, repo, "Bob", "Coworker")

	c, err := repo.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if c.Name != "Bob" {
		t.Errorf("expected 'Bob', got %q", c.Name)
	}
}

func TestContactGet_NotFound(t *testing.T) {
	repo := newTestContactRepo(t)

	_, err := repo.Get(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error for non-existent contact")
	}
}

func TestContactList_Empty(t *testing.T) {
	repo := newTestContactRepo(t)

	contacts, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(contacts) != 0 {
		t.Errorf("expected empty list, got %d items", len(contacts))
	}
}

func TestContactList_Ordered(t *testing.T) {
	repo := newTestContactRepo(t)

	seedContact(t, repo, "Zoe", "")
	seedContact(t, repo, "Alice", "")
	seedContact(t, repo, "Bob", "")

	contacts, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(contacts) != 3 {
		t.Fatalf("expected 3 contacts, got %d", len(contacts))
	}
	if contacts[0].Name != "Alice" {
		t.Errorf("expected first 'Alice', got %q", contacts[0].Name)
	}
	if contacts[1].Name != "Bob" {
		t.Errorf("expected second 'Bob', got %q", contacts[1].Name)
	}
	if contacts[2].Name != "Zoe" {
		t.Errorf("expected third 'Zoe', got %q", contacts[2].Name)
	}
}

func TestContactSearch(t *testing.T) {
	repo := newTestContactRepo(t)

	seedContact(t, repo, "Alpha", "")
	seedContact(t, repo, "Alpine", "")
	seedContact(t, repo, "Beta", "")

	results, err := repo.Search(context.Background(), "alp")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'alp', got %d", len(results))
	}
}

func TestContactSearch_CaseInsensitive(t *testing.T) {
	repo := newTestContactRepo(t)

	seedContact(t, repo, "Alpha", "")

	results, err := repo.Search(context.Background(), "ALPHA")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'ALPHA', got %d", len(results))
	}
}

func TestContactUpdate(t *testing.T) {
	repo := newTestContactRepo(t)
	id := seedContact(t, repo, "Old", "old notes")

	updated, err := repo.Update(context.Background(), id, "New", "new notes")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "New" {
		t.Errorf("expected 'New', got %q", updated.Name)
	}
	if updated.Notes != "new notes" {
		t.Errorf("expected 'new notes', got %q", updated.Notes)
	}
}

func TestContactFindByName(t *testing.T) {
	repo := newTestContactRepo(t)
	seedContact(t, repo, "Unique", "")

	c, err := repo.FindByName(context.Background(), "Unique")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if c.Name != "Unique" {
		t.Errorf("expected 'Unique', got %q", c.Name)
	}
}

func TestContactFindByName_NotFound(t *testing.T) {
	repo := newTestContactRepo(t)

	_, err := repo.FindByName(context.Background(), "NonExistent")
	if err == nil {
		t.Fatal("expected error for non-existent name")
	}
}

func TestContactDelete(t *testing.T) {
	repo := newTestContactRepo(t)
	id := seedContact(t, repo, "DeleteMe", "")

	if err := repo.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.Get(context.Background(), id)
	if err == nil {
		t.Error("expected error after delete")
	}
}
