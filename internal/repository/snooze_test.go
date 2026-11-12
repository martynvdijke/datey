package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestSnoozeRepo(t *testing.T) *SnoozeRepository {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_snooze_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewSnoozeRepository(client)
}

var testDay = time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)

func TestSnoozeUpsertCreatesThenUpdates(t *testing.T) {
	repo := newTestSnoozeRepo(t)
	ctx := context.Background()
	now := time.Now()

	if err := repo.Upsert(ctx, 7, testDay, 2, now.Add(24*time.Hour)); err != nil {
		t.Fatalf("Upsert(create) error = %v", err)
	}
	active, err := repo.Active(ctx, 7, testDay, 2, now)
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	if !active {
		t.Fatal("Active() = false after Upsert")
	}

	// Same key, longer window: one row, updated in place.
	if err := repo.Upsert(ctx, 7, testDay, 2, now.Add(3*24*time.Hour)); err != nil {
		t.Fatalf("Upsert(update) error = %v", err)
	}
	rows, err := repo.ListActive(ctx, now)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (upsert must not duplicate)", len(rows))
	}
	if got, want := rows[0].SnoozedUntil, now.Add(3*24*time.Hour); !got.Equal(want) {
		t.Errorf("SnoozedUntil = %v, want %v", got, want)
	}
}

func TestSnoozeActiveRespectsExpiry(t *testing.T) {
	repo := newTestSnoozeRepo(t)
	ctx := context.Background()
	now := time.Now()
	if err := repo.Upsert(ctx, 1, testDay, 0, now.Add(time.Hour)); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	if active, _ := repo.Active(ctx, 1, testDay, 0, now); !active {
		t.Error("Active() = false before expiry")
	}
	if active, _ := repo.Active(ctx, 1, testDay, 0, now.Add(2*time.Hour)); active {
		t.Error("Active() = true after expiry")
	}
	// A different user or a different occurrence is unaffected.
	if active, _ := repo.Active(ctx, 1, testDay, 5, now); active {
		t.Error("Active() = true for another user")
	}
	if active, _ := repo.Active(ctx, 1, testDay.AddDate(1, 0, 0), 0, now); active {
		t.Error("Active() = true for another occurrence")
	}
}

func TestSnoozeNormalizesOccasionToUTCDay(t *testing.T) {
	repo := newTestSnoozeRepo(t)
	ctx := context.Background()
	now := time.Now()

	// 23:30 in a +02:00 zone is still the same calendar day in UTC here; the
	// stored key must match a plain midnight-UTC lookup.
	occasion := time.Date(2026, 3, 14, 23, 30, 0, 0, time.FixedZone("CEST", 2*3600))
	if err := repo.Upsert(ctx, 3, occasion, 0, now.Add(24*time.Hour)); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	active, err := repo.Active(ctx, 3, testDay, 0, now)
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	if !active {
		t.Error("Active() = false: occasion was not normalized to a UTC day")
	}
}

func TestSnoozeDelete(t *testing.T) {
	repo := newTestSnoozeRepo(t)
	ctx := context.Background()
	now := time.Now()

	if err := repo.Delete(ctx, 9, testDay, 0); err != nil {
		t.Fatalf("Delete(missing) error = %v", err)
	}
	if err := repo.Upsert(ctx, 9, testDay, 0, now.Add(time.Hour)); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if err := repo.Delete(ctx, 9, testDay, 0); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if active, _ := repo.Active(ctx, 9, testDay, 0, now); active {
		t.Error("Active() = true after Delete")
	}
}

func TestSnoozeListActive(t *testing.T) {
	repo := newTestSnoozeRepo(t)
	ctx := context.Background()
	now := time.Now()

	if err := repo.Upsert(ctx, 1, testDay, 0, now.Add(time.Hour)); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if err := repo.Upsert(ctx, 2, testDay, 0, now.Add(-time.Hour)); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	rows, err := repo.ListActive(ctx, now)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(rows) != 1 || rows[0].EventID != 1 {
		t.Errorf("ListActive() = %v, want only event 1", rows)
	}
}
