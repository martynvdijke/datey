package scheduler

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

func TestParseChannelTargets(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantNil  bool
		wantErr  bool
		want     []string
	}{
		{
			name:    "empty string (pre-migration default)",
			raw:     "",
			wantNil: true,
			wantErr: false,
		},
		{
			name:    "null JSON",
			raw:     "null",
			wantNil: true,
			wantErr: false,
		},
		{
			name:    "empty JSON array",
			raw:     "[]",
			want:    []string{},
			wantErr: false,
		},
		{
			name:    "valid single channel",
			raw:     `["email"]`,
			want:    []string{"email"},
			wantErr: false,
		},
		{
			name:    "valid multi channel",
			raw:     `["email","gotify"]`,
			want:    []string{"email", "gotify"},
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			raw:     "{bad",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChannelTargets(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseChannelTargets() error = %v, wantErr = %v", err, tt.wantErr)
				return
			}
			if tt.wantNil && got != nil {
				t.Errorf("parseChannelTargets() = %v, want nil", got)
				return
			}
			if !tt.wantNil && tt.want != nil {
				if len(got) != len(tt.want) {
					t.Errorf("parseChannelTargets() = %v, want %v", got, tt.want)
					return
				}
				for i := range tt.want {
					if got[i] != tt.want[i] {
						t.Errorf("parseChannelTargets() = %v, want %v", got, tt.want)
						break
					}
				}
			}
		})
	}
}

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
	_, err := repo.Create(ctx, "past notification", past, []string{"test"}, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create one due in the future (should NOT be processed)
	future := time.Now().Add(24 * time.Hour)
	_, err = repo.Create(ctx, "future notification", future, []string{"test"}, nil)
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

func TestOneTimeScheduler_FallbackFromEmptyChannelTargets(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_ots_fallback?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)
	deliveryRepo := repository.NewNotificationDeliveryRepository(client)

	reg := notifier.NewRegistry()
	rec := newRecordingNotifier("test", true)
	reg.Register(rec)

	// Simulate a pre-migration notification: create via raw ENT client so
	// ChannelTargets is left at its default empty string, and no delivery
	// records exist.
	past := time.Now().Add(-1 * time.Hour)
	n, err := client.OneTimeNotification.Create().
		SetMessage("pre-migration notification").
		SetScheduledAt(past).
		SetCreatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("Create raw notification failed: %v", err)
	}
	if n.ChannelTargets != "" {
		t.Fatalf("expected empty ChannelTargets, got %q", n.ChannelTargets)
	}

	sched := NewOneTimeNotificationScheduler(repo, deliveryRepo, reg)
	sched.processDue(ctx)

	sent := rec.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 notification sent via fallback (all configured), got %d", len(sent))
	}

	// Verify the notification was marked as sent
	got, err := repo.Get(ctx, n.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != "sent" {
		t.Errorf("expected status 'sent', got %q", got.Status)
	}
}

func TestOneTimeScheduler_FallbackFromInvalidJSON(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_ots_fallback_invalid?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)
	deliveryRepo := repository.NewNotificationDeliveryRepository(client)

	reg := notifier.NewRegistry()
	rec := newRecordingNotifier("test", true)
	reg.Register(rec)

	// Simulate a notification with corrupted channel_targets
	past := time.Now().Add(-1 * time.Hour)
	n, err := client.OneTimeNotification.Create().
		SetMessage("corrupted notification").
		SetScheduledAt(past).
		SetChannelTargets("{bad json}").
		SetCreatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("Create raw notification failed: %v", err)
	}

	sched := NewOneTimeNotificationScheduler(repo, deliveryRepo, reg)
	sched.processDue(ctx)

	sent := rec.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 notification sent via fallback, got %d", len(sent))
	}

	// Verify the notification was marked as sent
	got, err := repo.Get(ctx, n.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != "sent" {
		t.Errorf("expected status 'sent', got %q", got.Status)
	}
}

func TestOneTimeScheduler_NoChannelsFallsbackToConfigured(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_ots_nochan?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)
	deliveryRepo := repository.NewNotificationDeliveryRepository(client)

	reg := notifier.NewRegistry()
	rec := newRecordingNotifier("test", true)
	inactive := newRecordingNotifier("inactive", false)
	reg.Register(rec)
	reg.Register(inactive)

	// Create notification with empty channel list
	past := time.Now().Add(-1 * time.Hour)
	_, err := repo.Create(ctx, "no channel specified", past, []string{}, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	sched := NewOneTimeNotificationScheduler(repo, deliveryRepo, reg)
	sched.processDue(ctx)

	sent := rec.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 notification sent via fallback to configured, got %d", len(sent))
	}
}

type failingNotifier struct {
	name   string
	active bool
}

func newFailingNotifier(name string, active bool) *failingNotifier {
	return &failingNotifier{name: name, active: active}
}

func (n *failingNotifier) Send(_ context.Context, title, message string) error {
	return fmt.Errorf("simulated failure for %s", n.name)
}

func (n *failingNotifier) Name() string { return n.name }

func (n *failingNotifier) IsConfigured() bool { return n.active }

func TestOneTimeScheduler_UpdatesDeliveryRecordsOnSuccess(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_ots_delivery_success?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)
	deliveryRepo := repository.NewNotificationDeliveryRepository(client)

	reg := notifier.NewRegistry()
	rec := newRecordingNotifier("test", true)
	reg.Register(rec)

	past := time.Now().Add(-1 * time.Hour)
	n, err := repo.Create(ctx, "delivery success test", past, []string{"test"}, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	sched := NewOneTimeNotificationScheduler(repo, deliveryRepo, reg)
	sched.processDue(ctx)

	// Check delivery records were updated to sent
	deliveries, err := deliveryRepo.ListByNotification(ctx, n.ID)
	if err != nil {
		t.Fatalf("ListByNotification failed: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery record, got %d", len(deliveries))
	}
	if deliveries[0].Status != "sent" {
		t.Errorf("expected delivery status 'sent', got '%s'", deliveries[0].Status)
	}
	if deliveries[0].SentAt == nil {
		t.Error("expected delivery sent_at to be set, got nil")
	}
}

func TestOneTimeScheduler_UpdatesDeliveryRecordsOnFailure(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_ots_delivery_fail?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	repo := repository.NewOneTimeNotificationRepository(client)
	deliveryRepo := repository.NewNotificationDeliveryRepository(client)

	reg := notifier.NewRegistry()
	fail := newFailingNotifier("failbot", true)
	reg.Register(fail)

	past := time.Now().Add(-1 * time.Hour)
	n, err := repo.Create(ctx, "delivery fail test", past, []string{"test"}, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	sched := NewOneTimeNotificationScheduler(repo, deliveryRepo, reg)
	sched.processDue(ctx)

	// Check delivery records were updated to failed
	deliveries, err := deliveryRepo.ListByNotification(ctx, n.ID)
	if err != nil {
		t.Fatalf("ListByNotification failed: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery record, got %d", len(deliveries))
	}
	if deliveries[0].Status != "failed" {
		t.Errorf("expected delivery status 'failed', got '%s'", deliveries[0].Status)
	}
	if deliveries[0].ErrorMessage == "" {
		t.Error("expected delivery error_message to be set, got empty")
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
	_, err := repo.Create(ctx, "future notification", future, []string{"test"}, nil)
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
