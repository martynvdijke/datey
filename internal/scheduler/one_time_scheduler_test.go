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

type recordingNotifier struct {
	mu     sync.Mutex
	sent   []string
	name   string
	active bool
}

func newRecordingNotifier(name string, active bool) *recordingNotifier {
	return &recordingNotifier{name: name, active: active}
}

func (n *recordingNotifier) Send(_ context.Context, title, message string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, title+": "+message)
	return nil
}

func (n *recordingNotifier) Name() string { return n.name }

func (n *recordingNotifier) IsConfigured() bool { return n.active }

func (n *recordingNotifier) Sent() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	result := make([]string, len(n.sent))
	copy(result, n.sent)
	return result
}

func TestOneTimeScheduler_ProcessesDueNotifications(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_ots_process?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)
	deliveryRepo := repository.NewNotificationDeliveryRepository(client)

	reg := notifier.NewRegistry()
	rec := newRecordingNotifier("test", true)
	reg.Register(rec)

	// Create a notification due in the past
	past := time.Now().Add(-1 * time.Hour)
	_, err := repo.Create(ctx, "past notification", past, []string{"test"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create one due in the future (should NOT be processed)
	future := time.Now().Add(24 * time.Hour)
	_, err = repo.Create(ctx, "future notification", future, []string{"test"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	sched := NewOneTimeNotificationScheduler(repo, deliveryRepo, reg)
	sched.processDue(ctx)

	sent := rec.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 notification sent, got %d", len(sent))
	}

	// Check the past notification was marked as sent
	due, err := repo.ListDue(ctx)
	if err != nil {
		t.Fatalf("ListDue failed: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("expected 0 due notifications after processing, got %d", len(due))
	}

	// Check future notification is still pending
	futureNotifs, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	for _, n := range futureNotifs {
		if n.Message == "future notification" && n.Status != "pending" {
			t.Errorf("future notification should still be pending, got %s", n.Status)
		}
	}
}

func TestOneTimeScheduler_NoDueNotifications(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_ots_none?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)
	deliveryRepo := repository.NewNotificationDeliveryRepository(client)

	reg := notifier.NewRegistry()
	rec := newRecordingNotifier("test", true)
	reg.Register(rec)

	// Only future notifications
	future := time.Now().Add(24 * time.Hour)
	_, err := repo.Create(ctx, "future notification", future, []string{"test"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	sched := NewOneTimeNotificationScheduler(repo, deliveryRepo, reg)
	sched.processDue(ctx)

	sent := rec.Sent()
	if len(sent) != 0 {
		t.Errorf("expected 0 notifications sent, got %d", len(sent))
	}
}
