package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/giftidea"
	_ "github.com/mattn/go-sqlite3"
)

func newStatusTestRepo(t *testing.T) *GiftIdeaRepository {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_gift_status?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewGiftIdeaRepository(client)
}

// legacyTimestamp overwrites a lifecycle field directly, so repeat-transition
// tests can prove an existing stamp is preserved rather than rewritten.
func legacyTimestamp(t *testing.T, repo *GiftIdeaRepository, id int, field string, at time.Time) {
	t.Helper()
	upd := repo.client.GiftIdea.UpdateOneID(id)
	switch field {
	case giftidea.FieldPurchasedAt:
		upd = upd.SetPurchasedAt(at)
	case giftidea.FieldGivenAt:
		upd = upd.SetGivenAt(at)
	case giftidea.FieldArchivedAt:
		upd = upd.SetArchivedAt(at)
	}
	if _, err := upd.Save(context.Background()); err != nil {
		t.Fatalf("set %s: %v", field, err)
	}
}

func TestSetStatusStampsLifecycle(t *testing.T) {
	repo := newStatusTestRepo(t)
	ctx := context.Background()
	personID := mustPerson(t, repo, "Gift Person")

	g, err := repo.Create(ctx, personID, "Bike", "", nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.PurchasedAt != nil || g.GivenAt != nil || g.ArchivedAt != nil {
		t.Fatalf("fresh idea should carry no lifecycle timestamps: %+v", g)
	}

	before := time.Now().Add(-time.Second)
	purchased, err := repo.SetStatus(ctx, g.ID, giftidea.StatusPurchased)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	if purchased.PurchasedAt == nil {
		t.Fatal("purchased_at not stamped on idea -> purchased")
	}
	if purchased.PurchasedAt.Before(before) {
		t.Fatalf("purchased_at not recent: %v", purchased.PurchasedAt)
	}

	// Repeat transition must not overwrite the stamp.
	oldPurchase := time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)
	legacyTimestamp(t, repo, g.ID, giftidea.FieldPurchasedAt, oldPurchase)
	again, err := repo.SetStatus(ctx, g.ID, giftidea.StatusPurchased)
	if err != nil {
		t.Fatalf("repeat purchase: %v", err)
	}
	if !again.PurchasedAt.Equal(oldPurchase) {
		t.Fatalf("repeat purchase overwrote purchased_at: %v", again.PurchasedAt)
	}

	// given stamps GivenAt and preserves the earlier purchase.
	given, err := repo.SetStatus(ctx, g.ID, giftidea.StatusGiven)
	if err != nil {
		t.Fatalf("give: %v", err)
	}
	if given.GivenAt == nil {
		t.Fatal("given_at not stamped on purchased -> given")
	}
	if !given.PurchasedAt.Equal(oldPurchase) {
		t.Fatalf("given cleared purchased_at: %v", given.PurchasedAt)
	}
	firstGiven := *given.GivenAt

	// Repeat given keeps the original given date.
	oldGiven := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	legacyTimestamp(t, repo, g.ID, giftidea.FieldGivenAt, oldGiven)
	againGiven, err := repo.SetStatus(ctx, g.ID, giftidea.StatusGiven)
	if err != nil {
		t.Fatalf("repeat given: %v", err)
	}
	if !againGiven.GivenAt.Equal(oldGiven) {
		t.Fatalf("repeat given overwrote given_at: %v", againGiven.GivenAt)
	}
	_ = firstGiven

	// archived stamps ArchivedAt and keeps given/purchased.
	archived, err := repo.SetStatus(ctx, g.ID, giftidea.StatusArchived)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("archived_at not stamped on given -> archived")
	}
	if !archived.GivenAt.Equal(oldGiven) || !archived.PurchasedAt.Equal(oldPurchase) {
		t.Fatalf("archive dropped earlier stamps: %+v", archived)
	}

	// Reverting to idea clears the whole lifecycle.
	reverted, err := repo.SetStatus(ctx, g.ID, giftidea.StatusIdea)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}
	if reverted.PurchasedAt != nil || reverted.GivenAt != nil || reverted.ArchivedAt != nil {
		t.Fatalf("revert to idea left timestamps: %+v", reverted)
	}
}

func TestUpdateStampsStatusChange(t *testing.T) {
	repo := newStatusTestRepo(t)
	ctx := context.Background()
	personID := mustPerson(t, repo, "Edit Person")
	g, err := repo.Create(ctx, personID, "Book", "", nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := repo.Update(ctx, g.ID, "Book 2", "note", nil, "", giftidea.StatusPurchased)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.PurchasedAt == nil {
		t.Fatal("edit path did not stamp purchased_at")
	}
	old := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)
	legacyTimestamp(t, repo, g.ID, giftidea.FieldPurchasedAt, old)
	// Editing without changing status must keep the original stamp.
	kept, err := repo.Update(ctx, g.ID, "Book 3", "", nil, "", giftidea.StatusPurchased)
	if err != nil {
		t.Fatalf("update again: %v", err)
	}
	if !kept.PurchasedAt.Equal(old) {
		t.Fatalf("edit overwrote purchased_at: %v", kept.PurchasedAt)
	}
}

func mustPerson(t *testing.T, repo *GiftIdeaRepository, name string) int {
	t.Helper()
	p, err := NewPersonRepository(repo.client).Create(context.Background(), name, "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	return p.ID
}

func TestMonthlySpendGrouping(t *testing.T) {
	repo := newStatusTestRepo(t)
	ctx := context.Background()
	personID := mustPerson(t, repo, "Spend Person")
	loc := time.FixedZone("plus2", 2*3600)
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, loc)

	mk := func(title string, status giftidea.Status, cents int, purchased, given *time.Time) {
		t.Helper()
		b := repo.client.GiftIdea.Create().
			SetTitle(title).
			SetStatus(status).
			SetCreatedAt(now).SetUpdatedAt(now).
			SetPersonID(personID).
			SetPriceCents(cents)
		if purchased != nil {
			b = b.SetPurchasedAt(*purchased)
		}
		if given != nil {
			b = b.SetGivenAt(*given)
		}
		if _, err := b.Save(ctx); err != nil {
			t.Fatalf("seed %s: %v", title, err)
		}
	}
	at := func(y int, m time.Month, d, h int) *time.Time {
		t := time.Date(y, m, d, h, 0, 0, 0, loc)
		return &t
	}

	// Given wins over purchased: January given (10.00), purchased in December.
	mk("given wins", giftidea.StatusGiven, 1000, at(2025, 12, 10, 9), at(2026, 1, 5, 9))
	// Purchased only, February.
	mk("purchased only", giftidea.StatusPurchased, 2000, at(2026, 2, 20, 9), nil)
	// Archived with a purchase date but no given date still counts as spend.
	mk("archived purchase", giftidea.StatusArchived, 500, at(2026, 2, 21, 9), nil)
	// No timestamp at all -> undated.
	mk("undated", giftidea.StatusGiven, 300, nil, nil)
	// Idea is not spend and never bucketed.
	mk("idea", giftidea.StatusIdea, 999, nil, nil)
	// Older than the 12-month window is omitted.
	mk("too old", giftidea.StatusGiven, 700, nil, at(2024, 1, 1, 0))
	// Month boundary in the instance zone: 2026-01-31T23:30Z is Feb 1 in +2.
	boundary := time.Date(2026, 1, 31, 23, 30, 0, 0, time.UTC)
	mk("boundary", giftidea.StatusGiven, 100, nil, &boundary)

	ideas, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	buckets := MonthlySpend(ideas, now, 12, loc)

	got := map[string]GiftMonthSpend{}
	var order []string
	for _, b := range buckets {
		key := b.Month.Format("2006-01")
		if b.Undated {
			key = "undated"
		}
		got[key] = b
		order = append(order, key)
	}

	if b, ok := got["2026-01"]; !ok || b.Cents != 1000 || b.Count != 1 {
		t.Errorf("January bucket = %+v, want 1000/1 (given wins over purchase)", b)
	}
	if b, ok := got["2026-02"]; !ok || b.Cents != 2600 || b.Count != 3 {
		t.Errorf("February bucket = %+v, want 2600/3 (purchased + archived + boundary)", b)
	}
	if b, ok := got["undated"]; !ok || b.Cents != 300 || b.Count != 1 {
		t.Errorf("undated bucket = %+v, want 300/1", b)
	}
	if _, ok := got["2024-01"]; ok {
		t.Error("bucket older than the window should be omitted")
	}
	if len(buckets) != 3 {
		t.Errorf("expected exactly 3 buckets, got %d (%v)", len(buckets), order)
	}
	// Chronological order, undated last.
	if order[len(order)-1] != "undated" {
		t.Errorf("undated bucket should be last, got %v", order)
	}
}
