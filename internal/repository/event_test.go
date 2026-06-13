package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestEventRepo(t *testing.T) (*EventRepository, *ContactRepository) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_event_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewEventRepository(client), NewContactRepository(client)
}

func seedContactForEvent(t *testing.T, contactRepo *ContactRepository, name string) int {
	t.Helper()
	c, err := contactRepo.Create(context.Background(), name, "")
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	return c.ID
}

func TestEventCreate(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Event Tester")

	date := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	e, err := eventRepo.Create(context.Background(), contactID, "birthday", date, "Fourth of July")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if e.Type != "birthday" {
		t.Errorf("expected type 'birthday', got %q", e.Type)
	}
	if !e.Date.Equal(date) {
		t.Errorf("expected date %v, got %v", date, e.Date)
	}
	if e.Description != "Fourth of July" {
		t.Errorf("expected 'Fourth of July', got %q", e.Description)
	}
}

func TestEventGet(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Get Test")

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e, err := eventRepo.Create(context.Background(), contactID, "anniversary", date, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := eventRepo.Get(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != e.ID {
		t.Errorf("expected ID %d, got %d", e.ID, got.ID)
	}
}

func TestEventList_Empty(t *testing.T) {
	eventRepo, _ := newTestEventRepo(t)

	events, err := eventRepo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty list, got %d items", len(events))
	}
}

func TestEventListByContact(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "List Test")

	// Create two events
	_, err := eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), "Event 1")
	if err != nil {
		t.Fatalf("create event 1: %v", err)
	}
	_, err = eventRepo.Create(context.Background(), contactID, "anniversary",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "Event 2")
	if err != nil {
		t.Fatalf("create event 2: %v", err)
	}

	events, err := eventRepo.ListByContact(context.Background(), contactID)
	if err != nil {
		t.Fatalf("ListByContact: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	// Should be ordered by date ascending
	if events[0].Date.After(events[1].Date) {
		t.Errorf("events should be ordered by date ascending")
	}
}

func TestEventListByContact_NoEvents(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Empty Contact")

	events, err := eventRepo.ListByContact(context.Background(), contactID)
	if err != nil {
		t.Fatalf("ListByContact: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty, got %d items", len(events))
	}
}

func TestEventListInRange(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Range Test")

	// Create events at various dates
	_, err := eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "Jan")
	if err != nil {
		t.Fatalf("create jan: %v", err)
	}
	_, err = eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), "Jun")
	if err != nil {
		t.Fatalf("create jun: %v", err)
	}
	_, err = eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC), "Dec")
	if err != nil {
		t.Fatalf("create dec: %v", err)
	}

	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	events, err := eventRepo.ListInRange(context.Background(), start, end)
	if err != nil {
		t.Fatalf("ListInRange: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event in range, got %d", len(events))
	}
	if events[0].Description != "Jun" {
		t.Errorf("expected 'Jun', got %q", events[0].Description)
	}
}

func TestEventUpdate(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Update Test")

	original := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e, err := eventRepo.Create(context.Background(), contactID, "birthday", original, "Original")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newDate := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	updated, err := eventRepo.Update(context.Background(), e.ID, "anniversary", newDate, "Updated")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Type != "anniversary" {
		t.Errorf("expected type 'anniversary', got %q", updated.Type)
	}
	if !updated.Date.Equal(newDate) {
		t.Errorf("expected date %v, got %v", newDate, updated.Date)
	}
	if updated.Description != "Updated" {
		t.Errorf("expected 'Updated', got %q", updated.Description)
	}
}

func TestEventDelete(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Delete Test")

	e, err := eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "to delete")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := eventRepo.Delete(context.Background(), e.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = eventRepo.Get(context.Background(), e.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestEventListUpcoming(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Upcoming Test")

	// Past event
	_, err := eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), "Past")
	if err != nil {
		t.Fatalf("create past: %v", err)
	}

	// Future event
	_, err = eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), "Future")
	if err != nil {
		t.Fatalf("create future: %v", err)
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)

	events, err := eventRepo.ListUpcoming(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ListUpcoming: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 upcoming event, got %d", len(events))
	}
	if events[0].Description != "Future" {
		t.Errorf("expected 'Future', got %q", events[0].Description)
	}
}
