package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	_ "github.com/mattn/go-sqlite3"
)

func newTestUserRepo(t *testing.T) *UserRepository {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_user_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewUserRepository(client)
}

func seedUser(t *testing.T, repo *UserRepository, username, passwordHash string, role user.Role) int {
	t.Helper()
	u, err := repo.Create(context.Background(), username, passwordHash, role)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u.ID
}

func TestUserCreate(t *testing.T) {
	repo := newTestUserRepo(t)

	u, err := repo.Create(context.Background(), "alice", "hash123", user.RoleUser)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Username != "alice" {
		t.Errorf("expected Username 'alice', got %q", u.Username)
	}
	if u.PasswordHash != "hash123" {
		t.Errorf("expected PasswordHash 'hash123', got %q", u.PasswordHash)
	}
	if u.Role != user.RoleUser {
		t.Errorf("expected Role 'user', got %q", u.Role)
	}
	if u.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
}

func TestUserCreate_AdminRole(t *testing.T) {
	repo := newTestUserRepo(t)

	u, err := repo.Create(context.Background(), "admin", "hash456", user.RoleAdmin)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Role != user.RoleAdmin {
		t.Errorf("expected Role 'admin', got %q", u.Role)
	}
}

func TestUserGetByID(t *testing.T) {
	repo := newTestUserRepo(t)
	id := seedUser(t, repo, "bob", "hash", user.RoleUser)

	u, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if u.Username != "bob" {
		t.Errorf("expected 'bob', got %q", u.Username)
	}
}

func TestUserGetByID_NotFound(t *testing.T) {
	repo := newTestUserRepo(t)

	_, err := repo.GetByID(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestUserGetByUsername(t *testing.T) {
	repo := newTestUserRepo(t)
	seedUser(t, repo, "charlie", "hash", user.RoleUser)

	u, err := repo.GetByUsername(context.Background(), "charlie")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if u.Username != "charlie" {
		t.Errorf("expected 'charlie', got %q", u.Username)
	}
}

func TestUserGetByUsername_NotFound(t *testing.T) {
	repo := newTestUserRepo(t)

	_, err := repo.GetByUsername(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent username")
	}
}

func TestUserList_Empty(t *testing.T) {
	repo := newTestUserRepo(t)

	users, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected empty list, got %d items", len(users))
	}
}

func TestUserList_Ordered(t *testing.T) {
	repo := newTestUserRepo(t)

	seedUser(t, repo, "zoe", "hash", user.RoleUser)
	seedUser(t, repo, "alice", "hash", user.RoleUser)
	seedUser(t, repo, "bob", "hash", user.RoleUser)

	users, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(users))
	}
	if users[0].Username != "alice" {
		t.Errorf("expected first 'alice', got %q", users[0].Username)
	}
	if users[1].Username != "bob" {
		t.Errorf("expected second 'bob', got %q", users[1].Username)
	}
	if users[2].Username != "zoe" {
		t.Errorf("expected third 'zoe', got %q", users[2].Username)
	}
}

func TestUserDelete(t *testing.T) {
	repo := newTestUserRepo(t)
	id := seedUser(t, repo, "deleteme", "hash", user.RoleUser)

	if err := repo.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.GetByID(context.Background(), id)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestUserExists_True(t *testing.T) {
	repo := newTestUserRepo(t)
	seedUser(t, repo, "exists", "hash", user.RoleUser)

	ok, err := repo.Exists(context.Background())
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !ok {
		t.Errorf("expected true, got false")
	}
}

func TestUserExists_False(t *testing.T) {
	repo := newTestUserRepo(t)

	ok, err := repo.Exists(context.Background())
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if ok {
		t.Errorf("expected false, got true")
	}
}
