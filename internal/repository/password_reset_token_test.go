package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	_ "github.com/mattn/go-sqlite3"
)

func newTestPasswordResetTokenRepo(t *testing.T) (*PasswordResetTokenRepository, *UserRepository) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_reset_token_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewPasswordResetTokenRepository(client), NewUserRepository(client)
}

func TestPasswordResetTokenCreate_GetByRawToken(t *testing.T) {
	repo, userRepo := newTestPasswordResetTokenRepo(t)
	ctx := context.Background()

	id := seedUser(t, userRepo, "alice", "hash", user.RoleUser)

	raw, expiresAt, err := repo.Create(ctx, id, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if raw == "" {
		t.Fatal("expected non-empty raw token")
	}
	if !expiresAt.After(time.Now()) {
		t.Errorf("expected future expiry, got %v", expiresAt)
	}

	tok, err := repo.GetByRawToken(ctx, raw)
	if err != nil {
		t.Fatalf("GetByRawToken: %v", err)
	}
	if tok.Edges.User == nil || tok.Edges.User.ID != id {
		t.Errorf("expected token to resolve to user %d", id)
	}
}

func TestPasswordResetToken_StoresHashOnly(t *testing.T) {
	repo, userRepo := newTestPasswordResetTokenRepo(t)
	ctx := context.Background()

	id := seedUser(t, userRepo, "alice", "hash", user.RoleUser)

	raw, _, err := repo.Create(ctx, id, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The raw token must never appear in the database; only its hash does.
	rows, err := repo.client.PasswordResetToken.Query().All(ctx)
	if err != nil {
		t.Fatalf("query tokens: %v", err)
	}
	for _, tok := range rows {
		if tok.TokenHash == raw {
			t.Error("raw token stored in database; expected hash only")
		}
		if tok.TokenHash == "" {
			t.Error("expected non-empty token hash")
		}
	}
}

func TestPasswordResetToken_UnknownRawToken(t *testing.T) {
	repo, _ := newTestPasswordResetTokenRepo(t)
	ctx := context.Background()

	if _, err := repo.GetByRawToken(ctx, "not-a-real-token"); err == nil {
		t.Fatal("expected error for unknown token")
	}
}

func TestPasswordResetToken_UsedTokenRejected(t *testing.T) {
	repo, userRepo := newTestPasswordResetTokenRepo(t)
	ctx := context.Background()

	id := seedUser(t, userRepo, "alice", "hash", user.RoleUser)

	raw, _, err := repo.Create(ctx, id, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	tok, err := repo.GetByRawToken(ctx, raw)
	if err != nil {
		t.Fatalf("GetByRawToken: %v", err)
	}
	if err := repo.MarkUsed(ctx, tok.ID); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}

	if _, err := repo.GetByRawToken(ctx, raw); err == nil {
		t.Error("expected used token to be rejected")
	}
}

func TestPasswordResetToken_ExpiredTokenRejected(t *testing.T) {
	repo, userRepo := newTestPasswordResetTokenRepo(t)
	ctx := context.Background()

	id := seedUser(t, userRepo, "alice", "hash", user.RoleUser)

	// Negative TTL → token is expired immediately.
	raw, _, err := repo.Create(ctx, id, -time.Minute)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := repo.GetByRawToken(ctx, raw); err == nil {
		t.Error("expected expired token to be rejected")
	}
}

func TestPasswordResetToken_NewCreateInvalidatesOld(t *testing.T) {
	repo, userRepo := newTestPasswordResetTokenRepo(t)
	ctx := context.Background()

	id := seedUser(t, userRepo, "alice", "hash", user.RoleUser)

	firstRaw, _, err := repo.Create(ctx, id, time.Hour)
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	secondRaw, _, err := repo.Create(ctx, id, time.Hour)
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if firstRaw == secondRaw {
		t.Fatal("expected distinct tokens")
	}

	// Only one token row may remain, and it must be the newer one.
	rows, err := repo.client.PasswordResetToken.Query().All(ctx)
	if err != nil {
		t.Fatalf("query tokens: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 token row after second create, got %d", len(rows))
	}
	if _, err := repo.GetByRawToken(ctx, firstRaw); err == nil {
		t.Error("expected older token to be invalidated")
	}
	if _, err := repo.GetByRawToken(ctx, secondRaw); err != nil {
		t.Errorf("expected newer token to resolve, got %v", err)
	}
}
