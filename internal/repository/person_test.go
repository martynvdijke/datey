package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestPersonRepo(t *testing.T) *PersonRepository {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_person_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewPersonRepository(client)
}

func seedPerson(t *testing.T, repo *PersonRepository, name, notes string) int {
	t.Helper()
	p, err := repo.Create(context.Background(), name, notes)
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}
	return p.ID
}

func TestPersonCreate(t *testing.T) {
	repo := newTestPersonRepo(t)

	p, err := repo.Create(context.Background(), "Alice", "Friend")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Name != "Alice" {
		t.Errorf("expected Name 'Alice', got %q", p.Name)
	}
	if p.Notes != "Friend" {
		t.Errorf("expected Notes 'Friend', got %q", p.Notes)
	}
	if p.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
}

func TestPersonGet(t *testing.T) {
	repo := newTestPersonRepo(t)
	id := seedPerson(t, repo, "Bob", "Coworker")

	p, err := repo.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.Name != "Bob" {
		t.Errorf("expected 'Bob', got %q", p.Name)
	}
}

func TestPersonGet_NotFound(t *testing.T) {
	repo := newTestPersonRepo(t)

	_, err := repo.Get(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error for non-existent person")
	}
}

func TestPersonList_Empty(t *testing.T) {
	repo := newTestPersonRepo(t)

	people, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(people) != 0 {
		t.Errorf("expected empty list, got %d items", len(people))
	}
}

func TestPersonList_Ordered(t *testing.T) {
	repo := newTestPersonRepo(t)

	seedPerson(t, repo, "Zoe", "")
	seedPerson(t, repo, "Alice", "")
	seedPerson(t, repo, "Bob", "")

	people, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(people) != 3 {
		t.Fatalf("expected 3 people, got %d", len(people))
	}
	if people[0].Name != "Alice" {
		t.Errorf("expected first 'Alice', got %q", people[0].Name)
	}
	if people[1].Name != "Bob" {
		t.Errorf("expected second 'Bob', got %q", people[1].Name)
	}
	if people[2].Name != "Zoe" {
		t.Errorf("expected third 'Zoe', got %q", people[2].Name)
	}
}

func TestPersonSearch(t *testing.T) {
	repo := newTestPersonRepo(t)

	seedPerson(t, repo, "Alpha", "")
	seedPerson(t, repo, "Alpine", "")
	seedPerson(t, repo, "Beta", "")

	results, err := repo.Search(context.Background(), "alp")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'alp', got %d", len(results))
	}
}

func TestPersonDelete(t *testing.T) {
	repo := newTestPersonRepo(t)
	id := seedPerson(t, repo, "DeleteMe", "")

	if err := repo.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.Get(context.Background(), id)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestPersonFindByName(t *testing.T) {
	repo := newTestPersonRepo(t)
	seedPerson(t, repo, "Unique", "")

	p, err := repo.FindByName(context.Background(), "Unique")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if p.Name != "Unique" {
		t.Errorf("expected 'Unique', got %q", p.Name)
	}
}

func TestPersonFindByName_NotFound(t *testing.T) {
	repo := newTestPersonRepo(t)

	_, err := repo.FindByName(context.Background(), "NonExistent")
	if err == nil {
		t.Fatal("expected error for non-existent name")
	}
}

func TestPersonUpdate(t *testing.T) {
	repo := newTestPersonRepo(t)
	id := seedPerson(t, repo, "Old", "old notes")

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
