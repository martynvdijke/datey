package repository

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/channelhealth"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

var channelHealthDBSeq atomic.Int64

func newTestChannelHealthRepo(t *testing.T) *ChannelHealthRepository {
	t.Helper()
	// A unique in-memory database per test: tests in this file assert on the
	// total row count, so they must not observe each other's rows.
	dsn := fmt.Sprintf("file:test_channel_health_%d?mode=memory&cache=shared&_fk=1", channelHealthDBSeq.Add(1))
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { client.Close() })
	return NewChannelHealthRepository(client)
}

func TestChannelHealth_RecordSuccessCreatesRow(t *testing.T) {
	repo := newTestChannelHealthRepo(t)
	ctx := context.Background()

	if err := repo.Upsert(ctx, "telegram", "chat-1", nil); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	rows, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Channel != "telegram" || row.Target != "chat-1" {
		t.Errorf("row key = (%q, %q)", row.Channel, row.Target)
	}
	if row.LastSuccessAt == nil {
		t.Error("LastSuccessAt = nil, want timestamp")
	}
	if row.LastFailureAt != nil {
		t.Errorf("LastFailureAt = %v, want nil", row.LastFailureAt)
	}
	if row.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", row.ConsecutiveFailures)
	}
}

func TestChannelHealth_FailuresIncrementAndSuccessResets(t *testing.T) {
	repo := newTestChannelHealthRepo(t)
	ctx := context.Background()
	boom := errors.New("smtp timeout")

	if err := repo.Upsert(ctx, "email", "a@example.com", boom); err != nil {
		t.Fatalf("first failure: %v", err)
	}
	if err := repo.Upsert(ctx, "email", "a@example.com", boom); err != nil {
		t.Fatalf("second failure: %v", err)
	}
	row, err := repo.client.ChannelHealth.Query().Where(channelhealth.ChannelEQ("email")).Only(ctx)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if row.ConsecutiveFailures != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", row.ConsecutiveFailures)
	}
	if row.LastError != "smtp timeout" {
		t.Errorf("LastError = %q", row.LastError)
	}
	if row.LastFailureAt == nil {
		t.Error("LastFailureAt = nil, want timestamp")
	}
	if row.LastSuccessAt != nil {
		t.Errorf("LastSuccessAt = %v, want nil", row.LastSuccessAt)
	}

	if err := repo.Upsert(ctx, "email", "a@example.com", nil); err != nil {
		t.Fatalf("success: %v", err)
	}
	row, err = repo.client.ChannelHealth.Query().Where(channelhealth.ChannelEQ("email")).Only(ctx)
	if err != nil {
		t.Fatalf("query after success: %v", err)
	}
	if row.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0 after success", row.ConsecutiveFailures)
	}
	if row.LastError != "" {
		t.Errorf("LastError = %q, want cleared", row.LastError)
	}
	if row.LastSuccessAt == nil {
		t.Error("LastSuccessAt = nil after success")
	}
	if row.LastFailureAt == nil {
		t.Error("LastFailureAt cleared, want the previous failure kept")
	}
}

func TestChannelHealth_TargetsAreIndependent(t *testing.T) {
	repo := newTestChannelHealthRepo(t)
	ctx := context.Background()

	if err := repo.RecordSuccess(ctx, "ntfy", "topic-a"); err != nil {
		t.Fatalf("success a: %v", err)
	}
	if err := repo.RecordFailure(ctx, "ntfy", "topic-b", "boom"); err != nil {
		t.Fatalf("failure b: %v", err)
	}

	rows, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	idx := IndexChannelHealth(rows)
	if got := idx[ChannelHealthKey("ntfy", "topic-a")]; got == nil || got.ConsecutiveFailures != 0 {
		t.Errorf("topic-a row = %+v", got)
	}
	if got := idx[ChannelHealthKey("ntfy", "topic-b")]; got == nil || got.ConsecutiveFailures != 1 {
		t.Errorf("topic-b row = %+v", got)
	}
	if got := idx[ChannelHealthKey("ntfy", "missing")]; got != nil {
		t.Errorf("unexpected row for missing target: %+v", got)
	}
}

func TestChannelHealth_IndexIsSingleLookup(t *testing.T) {
	rows := IndexChannelHealth(nil)
	if len(rows) != 0 {
		t.Errorf("index of empty slice = %v, want empty map", rows)
	}
}

func TestChannelHealth_RecordSuccessUsesInjectedClock(t *testing.T) {
	repo := newTestChannelHealthRepo(t)
	fixed := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)
	repo.Now = func() time.Time { return fixed }
	ctx := context.Background()

	if err := repo.RecordSuccess(ctx, "gotify", ""); err != nil {
		t.Fatalf("RecordSuccess() error = %v", err)
	}
	row, err := repo.client.ChannelHealth.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if row.LastSuccessAt == nil || !row.LastSuccessAt.Equal(fixed) {
		t.Errorf("LastSuccessAt = %v, want %v", row.LastSuccessAt, fixed)
	}
}
