package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestGroupNoteRepo(t *testing.T) (*GroupNoteRepository, *GroupRepository) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_groupnote_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewGroupNoteRepository(client), NewGroupRepository(client)
}

func TestGroupNoteCreateAndGet(t *testing.T) {
	noteRepo, groupRepo := newTestGroupNoteRepo(t)

	g, err := groupRepo.Create(context.Background(), "Family", "")
	if err != nil {
		t.Fatalf("Create group: %v", err)
	}

	date := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	n, err := noteRepo.Create(context.Background(), g.ID, "Reunion planned", date)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.Note != "Reunion planned" {
		t.Errorf("expected note text, got %q", n.Note)
	}

	got, err := noteRepo.Get(context.Background(), n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Note != "Reunion planned" {
		t.Errorf("expected 'Reunion planned', got %q", got.Note)
	}
}

func TestGroupNoteUpdateDelete(t *testing.T) {
	noteRepo, groupRepo := newTestGroupNoteRepo(t)

	g, _ := groupRepo.Create(context.Background(), "Club", "")
	n, err := noteRepo.Create(context.Background(), g.ID, "before", time.Now())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if _, err := noteRepo.Update(context.Background(), n.ID, "after", newDate); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := noteRepo.Get(context.Background(), n.ID)
	if got.Note != "after" {
		t.Errorf("expected updated note, got %q", got.Note)
	}

	if err := noteRepo.Delete(context.Background(), n.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := noteRepo.Get(context.Background(), n.ID); err == nil {
		t.Error("expected error after delete")
	}
}

func TestGroupNoteListByGroup(t *testing.T) {
	noteRepo, groupRepo := newTestGroupNoteRepo(t)

	gA, _ := groupRepo.Create(context.Background(), "A", "")
	gB, _ := groupRepo.Create(context.Background(), "B", "")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := base.AddDate(0, 0, 5)
	// Insert out of order to prove ordering.
	for _, tc := range []struct {
		groupID int
		text    string
		date    time.Time
	}{
		{gA.ID, "second", later},
		{gB.ID, "other-group", base},
		{gA.ID, "first", base},
	} {
		if _, err := noteRepo.Create(context.Background(), tc.groupID, tc.text, tc.date); err != nil {
			t.Fatalf("Create(%s): %v", tc.text, err)
		}
	}

	notes, err := noteRepo.ListByGroup(context.Background(), gA.ID)
	if err != nil {
		t.Fatalf("ListByGroup: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes for group A, got %d", len(notes))
	}
	if notes[0].Note != "first" || notes[1].Note != "second" {
		t.Errorf("expected date-ordered notes [first second], got [%s %s]", notes[0].Note, notes[1].Note)
	}
}

func TestGroupSetMembers(t *testing.T) {
	groupRepo, personRepo := newTestGroupRepo(t)

	g, _ := groupRepo.Create(context.Background(), "Team", "")
	p1, _ := personRepo.Create(context.Background(), "Alice", "", "")
	p2, _ := personRepo.Create(context.Background(), "Bob", "", "")
	p3, _ := personRepo.Create(context.Background(), "Carol", "", "")

	if err := groupRepo.SetMembers(context.Background(), g.ID, []int{p1.ID, p2.ID}); err != nil {
		t.Fatalf("SetMembers: %v", err)
	}
	members, _ := groupRepo.ListPeopleInGroup(context.Background(), g.ID)
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	// Replace membership entirely.
	if err := groupRepo.SetMembers(context.Background(), g.ID, []int{p3.ID}); err != nil {
		t.Fatalf("SetMembers replace: %v", err)
	}
	members, _ = groupRepo.ListPeopleInGroup(context.Background(), g.ID)
	if len(members) != 1 || members[0].Name != "Carol" {
		t.Fatalf("expected only Carol as member, got %d members", len(members))
	}

	// Empty list clears membership.
	if err := groupRepo.SetMembers(context.Background(), g.ID, nil); err != nil {
		t.Fatalf("SetMembers clear: %v", err)
	}
	members, _ = groupRepo.ListPeopleInGroup(context.Background(), g.ID)
	if len(members) != 0 {
		t.Errorf("expected 0 members after clear, got %d", len(members))
	}
}

func TestGroupListWithCounts_LoadsPeople(t *testing.T) {
	groupRepo, personRepo := newTestGroupRepo(t)

	g, _ := groupRepo.Create(context.Background(), "Zeta", "")
	p, _ := personRepo.Create(context.Background(), "Dana", "", "")
	if err := groupRepo.AddPerson(context.Background(), g.ID, p.ID); err != nil {
		t.Fatalf("AddPerson: %v", err)
	}

	groups, err := groupRepo.ListWithCounts(context.Background())
	if err != nil {
		t.Fatalf("ListWithCounts: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Edges.People) != 1 {
		t.Errorf("expected edges loaded with 1 person, got %d", len(groups[0].Edges.People))
	}
}
