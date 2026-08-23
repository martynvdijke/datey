package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/giftidea"
	_ "github.com/mattn/go-sqlite3"
)

func newTestGiftRepo(t *testing.T) (*GiftIdeaRepository, *PersonRepository) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_gift_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewGiftIdeaRepository(client), NewPersonRepository(client)
}

func TestGiftIdeaCreateList(t *testing.T) {
	repo, personRepo := newTestGiftRepo(t)
	ctx := context.Background()
	p, _ := personRepo.Create(ctx, "Alice", "", "")
	_, err := repo.Create(ctx, p.ID, "Book", "nice", nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	list, _ := repo.ListByPerson(ctx, p.ID)
	if len(list) != 1 || list[0].Title != "Book" || list[0].Status != giftidea.StatusIdea {
		t.Fatalf("list mismatch %v", list)
	}
}

func TestGiftIdeaFilterByStatus(t *testing.T) {
	repo, personRepo := newTestGiftRepo(t)
	ctx := context.Background()
	p, _ := personRepo.Create(ctx, "Bob", "", "")
	g1, _ := repo.Create(ctx, p.ID, "Idea1", "", nil, "")
	g2, _ := repo.Create(ctx, p.ID, "Idea2", "", nil, "")
	_, _ = repo.UpdateStatus(ctx, g2.ID, giftidea.StatusPurchased)
	_ = g1
	filtered, _ := repo.ListByPersonFiltered(ctx, p.ID, false)
	if len(filtered) != 1 || filtered[0].Title != "Idea1" {
		t.Fatalf("filtered expected 1 idea, got %v", filtered)
	}
	all, _ := repo.ListByPersonFiltered(ctx, p.ID, true)
	if len(all) != 2 {
		t.Fatalf("expected 2 with show purchased, got %d", len(all))
	}
}

func TestGiftIdeaCascadeOnPersonDelete(t *testing.T) {
	giftRepo, personRepo := newTestGiftRepo(t)
	ctx := context.Background()
	p, _ := personRepo.Create(ctx, "Carol", "", "")
	_, _ = giftRepo.Create(ctx, p.ID, "Gift", "", nil, "")
	if err := personRepo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("delete person: %v", err)
	}
	list, _ := giftRepo.ListByPerson(ctx, p.ID)
	if len(list) != 0 {
		t.Fatalf("expected cascade delete, got %d", len(list))
	}
}
