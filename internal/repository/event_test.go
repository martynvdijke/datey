package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestEventRepo(t *testing.T) (*EventRepository, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_event_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewEventRepository(client), client
}

func seedContactForEvent(t *testing.T, client *ent.Client, name string) int {
	t.Helper()
	c, err := client.Contact.Create().
		SetName(name).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
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

func TestEventListUpcomingOccurrences_AnnualHistoricalDate(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Occurrence Test")

	// A birthday stored with a historical birth year must surface on its
	// occurrence within the window (1990-05-12 → 2026-05-12).
	_, err := eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(1990, 5, 12, 0, 0, 0, 0, time.UTC), "Old Birthday")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	occs, err := eventRepo.ListUpcomingOccurrences(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ListUpcomingOccurrences: %v", err)
	}
	if len(occs) != 1 {
		t.Fatalf("expected 1 occurrence, got %d", len(occs))
	}
	want := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	if !occs[0].Date.Equal(want) {
		t.Errorf("expected occurrence %v, got %v", want, occs[0].Date)
	}
	if occs[0].Event.Description != "Old Birthday" {
		t.Errorf("expected event 'Old Birthday', got %q", occs[0].Event.Description)
	}
}

func TestEventListUpcomingOccurrences_CustomEventRecursAnnually(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "One-off Test")

	// Every event recurs annually, including custom types: a "meeting" stored
	// with a past date must surface on this year's occurrence.
	_, err := eventRepo.Create(context.Background(), contactID, "meeting",
		time.Date(2025, 8, 15, 0, 0, 0, 0, time.UTC), "One-off")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// A second event whose occurrence falls outside the window must not appear.
	_, err = eventRepo.Create(context.Background(), contactID, "meeting",
		time.Date(2030, 9, 15, 0, 0, 0, 0, time.UTC), "Far Off")
	if err != nil {
		t.Fatalf("create far: %v", err)
	}

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

	occs, err := eventRepo.ListUpcomingOccurrences(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ListUpcomingOccurrences: %v", err)
	}
	if len(occs) != 1 {
		t.Fatalf("expected 1 occurrence, got %d", len(occs))
	}
	want := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	if !occs[0].Date.Equal(want) {
		t.Errorf("expected occurrence %v, got %v", want, occs[0].Date)
	}
	if occs[0].Event.Description != "One-off" {
		t.Errorf("expected 'One-off', got %q", occs[0].Event.Description)
	}
}

func TestEventListUpcomingOccurrences_YearBoundary(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Boundary Test")

	// A January 5th event must surface with the NEXT year's date when the
	// window crosses the year boundary (Dec 25 → Jan 10).
	_, err := eventRepo.Create(context.Background(), contactID, "anniversary",
		time.Date(2010, 1, 5, 0, 0, 0, 0, time.UTC), "Jan Anniversary")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	from := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 10, 0, 0, 0, 0, time.UTC)

	occs, err := eventRepo.ListUpcomingOccurrences(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ListUpcomingOccurrences: %v", err)
	}
	if len(occs) != 1 {
		t.Fatalf("expected 1 occurrence, got %d", len(occs))
	}
	want := time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC)
	if !occs[0].Date.Equal(want) {
		t.Errorf("expected occurrence %v, got %v", want, occs[0].Date)
	}
}

func TestNextBirthdayOccurrence_NoEvents(t *testing.T) {
	eventRepo, _ := newTestEventRepo(t)

	got, err := eventRepo.NextBirthdayOccurrence(context.Background(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NextBirthdayOccurrence: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil with no birthday events, got %+v", got)
	}
}

func TestNextBirthdayOccurrence_EarliestWins(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Birthday Test")

	// Two historical birthdays: May 12 and August 20. From March 1 the next
	// occurrence is May 12.
	_, err := eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(1990, 8, 20, 0, 0, 0, 0, time.UTC), "Later")
	if err != nil {
		t.Fatalf("create later: %v", err)
	}
	_, err = eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(1990, 5, 12, 0, 0, 0, 0, time.UTC), "Soon")
	if err != nil {
		t.Fatalf("create soon: %v", err)
	}

	got, err := eventRepo.NextBirthdayOccurrence(context.Background(), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NextBirthdayOccurrence: %v", err)
	}
	if got == nil {
		t.Fatal("expected a birthday occurrence")
	}
	if want := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC); !got.Date.Equal(want) {
		t.Errorf("expected occurrence %v, got %v", want, got.Date)
	}
	if got.Event.Description != "Soon" {
		t.Errorf("expected event 'Soon', got %q", got.Event.Description)
	}
}

func TestNextBirthdayOccurrence_YearBoundaryAndLeapDay(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Boundary Test")

	// Jan 5 birthday with a from-date late in the year: the occurrence must
	// roll over to the next year.
	_, err := eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(2010, 1, 5, 0, 0, 0, 0, time.UTC), "Jan Birthday")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := eventRepo.NextBirthdayOccurrence(context.Background(), time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NextBirthdayOccurrence: %v", err)
	}
	if got == nil {
		t.Fatal("expected a birthday occurrence")
	}
	if want := time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC); !got.Date.Equal(want) {
		t.Errorf("expected occurrence %v, got %v", want, got.Date)
	}
}

func TestNextBirthdayOccurrence_LeapDayDrift(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Leap Test")

	// Feb 29 birthday from March 1, 2026 (non-leap): drifts to Feb 28, 2027.
	_, err := eventRepo.Create(context.Background(), contactID, "birthday",
		time.Date(1992, 2, 29, 0, 0, 0, 0, time.UTC), "Leap Birthday")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := eventRepo.NextBirthdayOccurrence(context.Background(), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NextBirthdayOccurrence: %v", err)
	}
	if got == nil {
		t.Fatal("expected a birthday occurrence")
	}
	if want := time.Date(2027, 2, 28, 0, 0, 0, 0, time.UTC); !got.Date.Equal(want) {
		t.Errorf("expected occurrence %v, got %v", want, got.Date)
	}
	if got.Event.Description != "Leap Birthday" {
		t.Errorf("expected event 'Leap Birthday', got %q", got.Event.Description)
	}
}

func TestNextBirthdayOccurrence_IgnoresNonBirthdays(t *testing.T) {
	eventRepo, contactRepo := newTestEventRepo(t)
	contactID := seedContactForEvent(t, contactRepo, "Filter Test")

	_, err := eventRepo.Create(context.Background(), contactID, "anniversary",
		time.Date(2010, 6, 1, 0, 0, 0, 0, time.UTC), "Anniversary")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := eventRepo.NextBirthdayOccurrence(context.Background(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NextBirthdayOccurrence: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil with only non-birthday events, got %+v", got)
	}
}

func TestEventCreateForGroupAndListByGroup(t *testing.T) {
	eventRepo, client := newTestEventRepo(t)
	ctx := context.Background()

	g, err := client.Group.Create().SetName("Crew").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).Save(ctx)
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}

	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	e1, err := eventRepo.CreateForGroup(ctx, g.ID, "trip", base.AddDate(0, 0, 3), "Later trip")
	if err != nil {
		t.Fatalf("CreateForGroup: %v", err)
	}
	if _, err := eventRepo.CreateForGroup(ctx, g.ID, "meeting", base, "First meeting"); err != nil {
		t.Fatalf("CreateForGroup second: %v", err)
	}

	events, err := eventRepo.ListByGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListByGroup: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 group events, got %d", len(events))
	}
	if events[0].ID != e1.ID && events[0].Type != "meeting" {
		t.Errorf("expected date-ascending order starting with meeting, got %q first", events[0].Type)
	}
	if events[0].Edges.Group == nil || events[0].Edges.Group.ID != g.ID {
		t.Error("expected group edge loaded on listed events")
	}

	// A person event must not appear in the group listing.
	p, err := client.Person.Create().SetName("Solo").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).Save(ctx)
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}
	if _, err := eventRepo.CreateForPerson(ctx, p.ID, "birthday", base, ""); err != nil {
		t.Fatalf("CreateForPerson: %v", err)
	}
	events, _ = eventRepo.ListByGroup(ctx, g.ID)
	if len(events) != 2 {
		t.Errorf("person event leaked into group list: got %d", len(events))
	}
}
