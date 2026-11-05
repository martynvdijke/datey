package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestPersonNoteRepo(t *testing.T) (*PersonNoteRepository, *PersonRepository) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_personnote_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewPersonNoteRepository(client), NewPersonRepository(client)
}

func TestPersonNoteCreateAndGet(t *testing.T) {
	noteRepo, personRepo := newTestPersonNoteRepo(t)

	p, err := personRepo.Create(context.Background(), "Alice", "", "")
	if err != nil {
		t.Fatalf("Create person: %v", err)
	}

	date := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	n, err := noteRepo.Create(context.Background(), p.ID, "Met at a conference", date)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.Note != "Met at a conference" {
		t.Errorf("expected note text, got %q", n.Note)
	}

	got, err := noteRepo.Get(context.Background(), p.ID, n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Note != "Met at a conference" {
		t.Errorf("expected 'Met at a conference', got %q", got.Note)
	}
}

func TestPersonNoteUpdateDelete(t *testing.T) {
	noteRepo, personRepo := newTestPersonNoteRepo(t)

	p, _ := personRepo.Create(context.Background(), "Bob", "", "")
	n, err := noteRepo.Create(context.Background(), p.ID, "before", time.Now())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if _, err := noteRepo.Update(context.Background(), p.ID, n.ID, "after", newDate); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := noteRepo.Get(context.Background(), p.ID, n.ID)
	if got.Note != "after" {
		t.Errorf("expected updated note, got %q", got.Note)
	}

	if err := noteRepo.Delete(context.Background(), p.ID, n.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := noteRepo.Get(context.Background(), p.ID, n.ID); err == nil {
		t.Error("expected error after delete")
	}
}

func TestPersonNoteListByPerson(t *testing.T) {
	noteRepo, personRepo := newTestPersonNoteRepo(t)

	pA, _ := personRepo.Create(context.Background(), "A", "", "")
	pB, _ := personRepo.Create(context.Background(), "B", "", "")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := base.AddDate(0, 0, 5)
	// Insert out of order to prove ordering.
	for _, tc := range []struct {
		personID int
		text     string
		date     time.Time
	}{
		{pA.ID, "second", later},
		{pB.ID, "other-person", base},
		{pA.ID, "first", base},
	} {
		if _, err := noteRepo.Create(context.Background(), tc.personID, tc.text, tc.date); err != nil {
			t.Fatalf("Create(%s): %v", tc.text, err)
		}
	}

	notes, err := noteRepo.ListByPerson(context.Background(), pA.ID)
	if err != nil {
		t.Fatalf("ListByPerson: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes for person A, got %d", len(notes))
	}
	if notes[0].Note != "first" || notes[1].Note != "second" {
		t.Errorf("expected date-ordered notes [first second], got [%s %s]", notes[0].Note, notes[1].Note)
	}
}

func TestPersonNoteCrossPersonIsolation(t *testing.T) {
	noteRepo, personRepo := newTestPersonNoteRepo(t)

	pA, _ := personRepo.Create(context.Background(), "Owner", "", "")
	pB, _ := personRepo.Create(context.Background(), "Stranger", "", "")
	n, err := noteRepo.Create(context.Background(), pA.ID, "private", time.Now())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := noteRepo.Get(context.Background(), pB.ID, n.ID); err == nil {
		t.Error("expected Get for another person to fail")
	}
	if _, err := noteRepo.Update(context.Background(), pB.ID, n.ID, "hijacked", time.Now()); err == nil {
		t.Error("expected Update for another person to fail")
	}
	if err := noteRepo.Delete(context.Background(), pB.ID, n.ID); err == nil {
		t.Error("expected Delete for another person to fail")
	}
	if _, err := noteRepo.Get(context.Background(), pA.ID, n.ID); err != nil {
		t.Error("note should still exist for its owner")
	}
}
