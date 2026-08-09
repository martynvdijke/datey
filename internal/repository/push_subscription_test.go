package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	_ "github.com/mattn/go-sqlite3"
)

func newTestPushSubscriptionRepo(t *testing.T) (*PushSubscriptionRepository, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_push_sub_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewPushSubscriptionRepository(client), client
}

func seedPushUser(t *testing.T, client *ent.Client, username string) int {
	t.Helper()
	u, err := client.User.Create().
		SetUsername(username).
		SetPasswordHash("hash").
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u.ID
}

func TestPushSubscriptionUpsert_CreatesAndReplaces(t *testing.T) {
	repo, client := newTestPushSubscriptionRepo(t)
	ctx := context.Background()
	uid := seedPushUser(t, client, "pushuser")

	if _, err := repo.Upsert(ctx, uid, "https://push.example.com/a", "p256dh-1", "auth-1"); err != nil {
		t.Fatalf("Upsert create: %v", err)
	}
	subs, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription after first upsert, got %d", len(subs))
	}
	if subs[0].P256dh != "p256dh-1" || subs[0].Auth != "auth-1" {
		t.Errorf("unexpected keys: p256dh=%q auth=%q", subs[0].P256dh, subs[0].Auth)
	}

	// Same endpoint again → replace, not duplicate.
	if _, err := repo.Upsert(ctx, uid, "https://push.example.com/a", "p256dh-2", "auth-2"); err != nil {
		t.Fatalf("Upsert replace: %v", err)
	}
	subs, err = repo.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll after replace: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription after replace, got %d", len(subs))
	}
	if subs[0].P256dh != "p256dh-2" || subs[0].Auth != "auth-2" {
		t.Errorf("keys not updated: p256dh=%q auth=%q", subs[0].P256dh, subs[0].Auth)
	}
}

func TestPushSubscriptionListByUser(t *testing.T) {
	repo, client := newTestPushSubscriptionRepo(t)
	ctx := context.Background()
	uidA := seedPushUser(t, client, "user-a")
	uidB := seedPushUser(t, client, "user-b")

	if _, err := repo.Upsert(ctx, uidA, "https://push.example.com/a", "k", "a"); err != nil {
		t.Fatalf("Upsert A: %v", err)
	}
	if _, err := repo.Upsert(ctx, uidB, "https://push.example.com/b", "k", "a"); err != nil {
		t.Fatalf("Upsert B: %v", err)
	}

	subs, err := repo.ListByUser(ctx, uidA)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(subs) != 1 || subs[0].Endpoint != "https://push.example.com/a" {
		t.Fatalf("expected only user A's subscription, got %d entries", len(subs))
	}
}

func TestPushSubscriptionDeleteByEndpoint(t *testing.T) {
	repo, client := newTestPushSubscriptionRepo(t)
	ctx := context.Background()
	uid := seedPushUser(t, client, "pushuser")

	if _, err := repo.Upsert(ctx, uid, "https://push.example.com/a", "k", "a"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := repo.DeleteByEndpoint(ctx, "https://push.example.com/a"); err != nil {
		t.Fatalf("DeleteByEndpoint: %v", err)
	}
	n, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 subscriptions after delete, got %d", n)
	}
}

func TestPushSubscriptionCount(t *testing.T) {
	repo, client := newTestPushSubscriptionRepo(t)
	ctx := context.Background()
	uid := seedPushUser(t, client, "pushuser")

	if n, err := repo.Count(ctx); err != nil || n != 0 {
		t.Fatalf("expected empty count, got %d (err=%v)", n, err)
	}
	if _, err := repo.Upsert(ctx, uid, "https://push.example.com/a", "k", "a"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	n, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
}
