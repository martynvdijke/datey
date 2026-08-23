package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestTagRepo(t *testing.T) (*TagRepository, *PersonRepository) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_tag_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewTagRepository(client), NewPersonRepository(client)
}

func TestTagNormalizeDedup(t *testing.T) {
	repo, _ := newTestTagRepo(t)
	ctx := context.Background()
	a, err := repo.FindOrCreate(ctx, "vip")
	if err != nil {
		t.Fatalf("FindOrCreate vip: %v", err)
	}
	b, err := repo.FindOrCreate(ctx, "VIP")
	if err != nil {
		t.Fatalf("FindOrCreate VIP: %v", err)
	}
	if a.ID != b.ID {
		t.Errorf("expected dedup, got %d vs %d", a.ID, b.ID)
	}
	c, err := repo.FindOrCreate(ctx, "  vip  ")
	if err != nil {
		t.Fatalf("FindOrCreate trim: %v", err)
	}
	if c.ID != a.ID {
		t.Errorf("expected trim dedup")
	}
	// invalid name
	if _, err := repo.FindOrCreate(ctx, "bad name!"); err == nil {
		t.Error("expected invalid tag error")
	}
}

func TestTagAttachDetach(t *testing.T) {
	tagRepo, personRepo := newTestTagRepo(t)
	ctx := context.Background()
	p, err := personRepo.Create(ctx, "Alice", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if err := tagRepo.AddToPerson(ctx, p.ID, "vip"); err != nil {
		t.Fatalf("AddToPerson: %v", err)
	}
	tags, err := tagRepo.ListByPerson(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListByPerson: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "vip" {
		t.Fatalf("expected vip tag, got %v", tags)
	}
	// add same tag again should not duplicate
	_ = tagRepo.AddToPerson(ctx, p.ID, "VIP")
	tags, _ = tagRepo.ListByPerson(ctx, p.ID)
	if len(tags) != 1 {
		t.Errorf("expected dedup attach, got %d", len(tags))
	}
	if err := tagRepo.RemoveFromPerson(ctx, p.ID, "vip"); err != nil {
		t.Fatalf("RemoveFromPerson: %v", err)
	}
	tags, _ = tagRepo.ListByPerson(ctx, p.ID)
	if len(tags) != 0 {
		t.Errorf("expected 0 after remove, got %d", len(tags))
	}
}

func TestTagSearchByPrefix(t *testing.T) {
	repo, _ := newTestTagRepo(t)
	ctx := context.Background()
	for _, n := range []string{"vip", "vip2", "summer-camp", "gift-needed"} {
		if _, err := repo.FindOrCreate(ctx, n); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	results, err := repo.SearchByPrefix(ctx, "vi", 10)
	if err != nil {
		t.Fatalf("SearchByPrefix: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 for vi, got %d", len(results))
	}
	// case insensitive
	results, _ = repo.SearchByPrefix(ctx, "VI", 10)
	if len(results) != 2 {
		t.Errorf("expected case-insensitive 2, got %d", len(results))
	}
	// limit
	results, _ = repo.SearchByPrefix(ctx, "", 2)
	if len(results) != 2 {
		t.Errorf("expected limit 2, got %d", len(results))
	}
	// alphabetical order
	if results[0].Name > results[1].Name {
		t.Errorf("expected alphabetical order")
	}
}

func TestTagListPeopleByTagsAND(t *testing.T) {
	tagRepo, personRepo := newTestTagRepo(t)
	ctx := context.Background()
	p1, _ := personRepo.Create(ctx, "P1", "", "")
	p2, _ := personRepo.Create(ctx, "P2", "", "")
	_ = tagRepo.AddToPerson(ctx, p1.ID, "vip")
	_ = tagRepo.AddToPerson(ctx, p1.ID, "camp")
	_ = tagRepo.AddToPerson(ctx, p2.ID, "vip")
	people, err := tagRepo.ListPeopleByTags(ctx, []string{"vip"})
	if err != nil {
		t.Fatalf("ListPeopleByTags: %v", err)
	}
	if len(people) != 2 {
		t.Fatalf("expected 2 for vip, got %d", len(people))
	}
	people, _ = tagRepo.ListPeopleByTags(ctx, []string{"vip", "camp"})
	if len(people) != 1 || people[0].ID != p1.ID {
		t.Fatalf("expected AND to return P1 only, got %v", people)
	}
}
