package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestAuditRepo(t *testing.T) (*AuditEntryRepository, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_audit_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewAuditEntryRepository(client), client
}

func TestAuditEntryAppendAndList(t *testing.T) {
	repo, _ := newTestAuditRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	e := &ent.AuditEntry{
		CreatedAt:     now,
		ActorUsername: "alice",
		Action:        "person.delete",
		Target:        "42",
		SourceIP:      "1.2.3.4",
	}
	saved, err := repo.Append(ctx, e)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if saved.ID == 0 {
		t.Error("expected non-zero ID")
	}
	list, err := repo.List(ctx, AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}
	if list[0].ActorUsername != "alice" || list[0].Action != "person.delete" || list[0].Target != "42" {
		t.Errorf("unexpected entry: %+v", list[0])
	}
}

func TestAuditEntryFilterActor(t *testing.T) {
	repo, _ := newTestAuditRepo(t)
	ctx := context.Background()
	now := time.Now()
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: now, ActorUsername: "alice", Action: "person.delete", Target: "1"})
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: now.Add(time.Second), ActorUsername: "bob", Action: "person.delete", Target: "2"})
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: now.Add(2 * time.Second), ActorUsername: "alice", Action: "user.create", Target: "3"})

	list, err := repo.List(ctx, AuditFilter{Actor: "alice"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 entries for alice, got %d", len(list))
	}
	for _, e := range list {
		if e.ActorUsername != "alice" {
			t.Errorf("expected alice, got %q", e.ActorUsername)
		}
	}
}

func TestAuditEntryFilterAction(t *testing.T) {
	repo, _ := newTestAuditRepo(t)
	ctx := context.Background()
	now := time.Now()
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: now, ActorUsername: "alice", Action: "person.delete", Target: "1"})
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: now.Add(time.Second), ActorUsername: "bob", Action: "user.create", Target: "2"})
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: now.Add(2 * time.Second), ActorUsername: "alice", Action: "person.delete", Target: "3"})

	list, err := repo.List(ctx, AuditFilter{Action: "person.delete"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 entries for person.delete, got %d", len(list))
	}
	for _, e := range list {
		if e.Action != "person.delete" {
			t.Errorf("expected person.delete, got %q", e.Action)
		}
	}
}

func TestAuditEntryFilterDateRange(t *testing.T) {
	repo, _ := newTestAuditRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: base, ActorUsername: "a", Action: "x", Target: "1"})
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: base.Add(24 * time.Hour), ActorUsername: "a", Action: "x", Target: "2"})
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: base.Add(48 * time.Hour), ActorUsername: "a", Action: "x", Target: "3"})

	from := base.Add(12 * time.Hour)
	to := base.Add(36 * time.Hour)
	list, err := repo.List(ctx, AuditFilter{From: &from, To: &to})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 entry in range, got %d", len(list))
	}
	if list[0].Target != "2" {
		t.Errorf("expected target 2, got %q", list[0].Target)
	}

	// From only
	list, _ = repo.List(ctx, AuditFilter{From: &from})
	if len(list) != 2 {
		t.Errorf("expected 2 entries from >= from, got %d", len(list))
	}
	// To only
	list, _ = repo.List(ctx, AuditFilter{To: &to})
	if len(list) != 2 {
		t.Errorf("expected 2 entries to <= to, got %d", len(list))
	}
}

func TestAuditEntryNewestFirstOrdering(t *testing.T) {
	repo, _ := newTestAuditRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		_, _ = repo.Append(ctx, &ent.AuditEntry{
			CreatedAt:     base.Add(time.Duration(i) * time.Hour),
			ActorUsername: "u",
			Action:        "a",
			Target:        string(rune('0' + i)),
		})
	}
	list, err := repo.List(ctx, AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 5 {
		t.Fatalf("expected 5, got %d", len(list))
	}
	for i := 0; i < 4; i++ {
		if !list[i].CreatedAt.After(list[i+1].CreatedAt) && !list[i].CreatedAt.Equal(list[i+1].CreatedAt) {
			t.Errorf("expected newest first: index %d %v not after %v", i, list[i].CreatedAt, list[i+1].CreatedAt)
		}
	}
}

func TestAuditEntryPruneToCap(t *testing.T) {
	repo, client := newTestAuditRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		_, _ = repo.Append(ctx, &ent.AuditEntry{
			CreatedAt:     base.Add(time.Duration(i) * time.Hour),
			ActorUsername: "u",
			Action:        "a",
			Target:        string(rune('0' + i)),
		})
	}
	n, err := repo.PruneToCap(ctx, 3)
	if err != nil {
		t.Fatalf("PruneToCap: %v", err)
	}
	if n != 2 {
		t.Errorf("expected delete 2, got %d", n)
	}
	count, _ := client.AuditEntry.Query().Count(ctx)
	if count != 3 {
		t.Errorf("expected count 3 after prune, got %d", count)
	}
	// Oldest entries (targets 0,1) should be removed, newest remain (2,3,4)
	list, _ := repo.List(ctx, AuditFilter{})
	targets := map[string]bool{}
	for _, e := range list {
		targets[e.Target] = true
	}
	if targets["0"] || targets["1"] {
		t.Error("oldest entries should have been removed")
	}
	if !targets["2"] || !targets["3"] || !targets["4"] {
		t.Error("newest entries should remain")
	}
	// Prune when already under cap should do nothing
	n, _ = repo.PruneToCap(ctx, 5)
	if n != 0 {
		t.Errorf("expected 0 deleted when under cap, got %d", n)
	}
}
