package datey_test

import (
	"context"
	"testing"
	"time"

	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func TestUpcomingEvents(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	contact, err := client.Contact.Create().
		SetName("Alice").
		SetNotes("").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}

	_, err = client.Event.Create().
		SetType("birthday").
		SetDate(time.Date(2026, time.July, 4, 0, 0, 0, 0, time.UTC)).
		SetDescription("test").
		SetCreatedAt(time.Now()).
		SetContact(contact).
		Save(context.Background())
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	events, err := client.Event.Query().WithContact().All(context.Background())
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	contact2, _ := e.Edges.ContactOrErr()
	if contact2.Name != "Alice" {
		t.Errorf("expected contact name Alice, got %s", contact2.Name)
	}
}
