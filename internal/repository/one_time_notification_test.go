package repository_test

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

func TestOneTimeNotificationRepository_CreateAndList(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_on_create?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)

	future := time.Now().Add(24 * time.Hour)
	n, err := repo.Create(ctx, "test message", future, []string{"email"}, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if n.Message != "test message" {
		t.Errorf("expected message 'test message', got '%s'", n.Message)
	}
	if n.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", n.Status)
	}
	if n.SentAt != nil {
		t.Errorf("expected sent_at nil for new notification, got %v", n.SentAt)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(list))
	}
	if list[0].ID != n.ID {
		t.Errorf("expected id %d, got %d", n.ID, list[0].ID)
	}
}

func TestOneTimeNotificationRepository_Get(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_on_get?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)

	future := time.Now().Add(24 * time.Hour)
	created, err := repo.Create(ctx, "get test", future, []string{"email"}, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Message != "get test" {
		t.Errorf("expected message 'get test', got '%s'", got.Message)
	}
}

func TestOneTimeNotificationRepository_Delete(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_on_delete?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)

	future := time.Now().Add(24 * time.Hour)
	created, err := repo.Create(ctx, "delete test", future, []string{"email"}, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List after delete failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 notifications after delete, got %d", len(list))
	}
}

func TestOneTimeNotificationRepository_ListDue(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_on_listdue?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	_, err := repo.Create(ctx, "past notification", past, []string{"email"}, nil)
	if err != nil {
		t.Fatalf("Create past failed: %v", err)
	}
	_, err = repo.Create(ctx, "future notification", future, []string{"email"}, nil)
	if err != nil {
		t.Fatalf("Create future failed: %v", err)
	}

	due, err := repo.ListDue(ctx)
	if err != nil {
		t.Fatalf("ListDue failed: %v", err)
	}

	if len(due) != 1 {
		t.Fatalf("expected 1 due notification, got %d", len(due))
	}
	if due[0].Message != "past notification" {
		t.Errorf("expected 'past notification', got '%s'", due[0].Message)
	}
}

func TestOneTimeNotificationRepository_MarkSent(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_on_marksent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)

	past := time.Now().Add(-1 * time.Hour)
	created, err := repo.Create(ctx, "mark sent test", past, []string{"email"}, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.MarkSent(ctx, created.ID); err != nil {
		t.Fatalf("MarkSent failed: %v", err)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after MarkSent failed: %v", err)
	}
	if got.Status != "sent" {
		t.Errorf("expected status 'sent', got '%s'", got.Status)
	}
	if got.SentAt == nil {
		t.Error("expected sent_at to be set, got nil")
	}
}

func TestOneTimeNotificationRepository_MarkFailed(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_on_markfailed?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)

	past := time.Now().Add(-1 * time.Hour)
	created, err := repo.Create(ctx, "mark failed test", past, []string{"email"}, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.MarkFailed(ctx, created.ID); err != nil {
		t.Fatalf("MarkFailed failed: %v", err)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after MarkFailed failed: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("expected status 'failed', got '%s'", got.Status)
	}
}
