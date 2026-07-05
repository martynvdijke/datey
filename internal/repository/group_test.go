package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestGroupRepo(t *testing.T) (*GroupRepository, *PersonRepository) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_group_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewGroupRepository(client), NewPersonRepository(client)
}

func TestGroupCreate(t *testing.T) {
	groupRepo, _ := newTestGroupRepo(t)

	g, err := groupRepo.Create(context.Background(), "Friends", "Close friends")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if g.Name != "Friends" {
		t.Errorf("expected Name 'Friends', got %q", g.Name)
	}
	if g.Description != "Close friends" {
		t.Errorf("expected Description 'Close friends', got %q", g.Description)
	}
}

func TestGroupList_Empty(t *testing.T) {
	groupRepo, _ := newTestGroupRepo(t)

	groups, err := groupRepo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("expected empty list, got %d items", len(groups))
	}
}

func TestGroupGetByID(t *testing.T) {
	groupRepo, _ := newTestGroupRepo(t)

	g, err := groupRepo.Create(context.Background(), "Family", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := groupRepo.GetByID(context.Background(), g.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Family" {
		t.Errorf("expected 'Family', got %q", got.Name)
	}
}

func TestGroupDelete(t *testing.T) {
	groupRepo, _ := newTestGroupRepo(t)

	g, err := groupRepo.Create(context.Background(), "Temp", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := groupRepo.Delete(context.Background(), g.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = groupRepo.GetByID(context.Background(), g.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestGroupAddRemovePerson(t *testing.T) {
	groupRepo, personRepo := newTestGroupRepo(t)

	g, err := groupRepo.Create(context.Background(), "Club", "")
	if err != nil {
		t.Fatalf("Create group: %v", err)
	}

	p, err := personRepo.Create(context.Background(), "Member", "", "")
	if err != nil {
		t.Fatalf("Create person: %v", err)
	}

	// Add person to group
	if err := groupRepo.AddPerson(context.Background(), g.ID, p.ID); err != nil {
		t.Fatalf("AddPerson: %v", err)
	}

	// ListByPerson
	groups, err := groupRepo.ListByPerson(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("ListByPerson: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "Club" {
		t.Errorf("expected 'Club', got %q", groups[0].Name)
	}

	// ListPeopleInGroup
	people, err := groupRepo.ListPeopleInGroup(context.Background(), g.ID)
	if err != nil {
		t.Fatalf("ListPeopleInGroup: %v", err)
	}
	if len(people) != 1 {
		t.Fatalf("expected 1 person in group, got %d", len(people))
	}
	if people[0].Name != "Member" {
		t.Errorf("expected 'Member', got %q", people[0].Name)
	}

	// Remove person from group
	if err := groupRepo.RemovePerson(context.Background(), g.ID, p.ID); err != nil {
		t.Fatalf("RemovePerson: %v", err)
	}

	people, err = groupRepo.ListPeopleInGroup(context.Background(), g.ID)
	if err != nil {
		t.Fatalf("ListPeopleInGroup after remove: %v", err)
	}
	if len(people) != 0 {
		t.Errorf("expected 0 people after removal, got %d", len(people))
	}
}
