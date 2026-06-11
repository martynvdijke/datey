package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

func TestOneTimeScheduler_FullFlow(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_ots_full?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)

	reg := notifier.NewRegistry()
	var mu sync.Mutex
	sentMessages := make([]string, 0)

	mock := &notifiable{
		name: "integration-test",
		sendFn: func(_ context.Context, title, message string) error {
			mu.Lock()
			defer mu.Unlock()
			sentMessages = append(sentMessages, title+": "+message)
			return nil
		},
		configured: true,
	}
	reg.Register(mock)

	// Create a notification due in the past (should fire immediately)
	past := time.Now().Add(-30 * time.Minute)
	created, err := repo.Create(ctx, "integration test message", past)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	sched := NewOneTimeNotificationScheduler(repo, reg)
	sched.processDue(ctx)

	// Verify the notification was sent
	mu.Lock()
	sentCount := len(sentMessages)
	sentMsg := ""
	if sentCount > 0 {
		sentMsg = sentMessages[0]
	}
	mu.Unlock()

	if sentCount != 1 {
		t.Fatalf("expected 1 notification sent, got %d", sentCount)
	}
	if !contains(sentMsg, "integration test message") {
		t.Errorf("expected message content in sent notification, got: %s", sentMsg)
	}

	// Verify status was updated to sent
	updated, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after processing failed: %v", err)
	}
	if updated.Status != "sent" {
		t.Errorf("expected status 'sent', got '%s'", updated.Status)
	}
	if updated.SentAt == nil {
		t.Error("expected sent_at to be set")
	}

	// Verify it's no longer due
	due, err := repo.ListDue(ctx)
	if err != nil {
		t.Fatalf("ListDue failed: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("expected 0 due notifications after processing, got %d", len(due))
	}
}

type notifiable struct {
	name       string
	sendFn     func(ctx context.Context, title, message string) error
	configured bool
}

func (n *notifiable) Send(ctx context.Context, title, message string) error {
	return n.sendFn(ctx, title, message)
}

func (n *notifiable) Name() string { return n.name }

func (n *notifiable) IsConfigured() bool { return n.configured }

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
