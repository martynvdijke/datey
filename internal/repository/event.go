package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/contact"
	"github.com/datey/datey/ent/event"
)

type EventRepository struct {
	client *ent.Client
}

func NewEventRepository(client *ent.Client) *EventRepository {
	return &EventRepository{client: client}
}

func (r *EventRepository) Create(ctx context.Context, contactID int, eventType string, date time.Time, description string) (*ent.Event, error) {
	return r.client.Event.Create().
		SetType(eventType).
		SetDate(date).
		SetDescription(description).
		SetCreatedAt(time.Now()).
		SetContactID(contactID).
		Save(ctx)
}

func (r *EventRepository) Get(ctx context.Context, id int) (*ent.Event, error) {
	return r.client.Event.Get(ctx, id)
}

func (r *EventRepository) List(ctx context.Context) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Order(ent.Asc(event.FieldDate)).
		All(ctx)
}

func (r *EventRepository) ListByContact(ctx context.Context, contactID int) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Where(event.HasContactWith(contact.IDEQ(contactID))).
		Order(ent.Asc(event.FieldDate)).
		All(ctx)
}

func (r *EventRepository) Update(ctx context.Context, id int, eventType string, date time.Time, description string) (*ent.Event, error) {
	return r.client.Event.UpdateOneID(id).
		SetType(eventType).
		SetDate(date).
		SetDescription(description).
		Save(ctx)
}

func (r *EventRepository) ListInRange(ctx context.Context, start, end time.Time) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Where(event.DateGTE(start), event.DateLTE(end)).
		Order(ent.Asc(event.FieldDate)).
		WithContact().
		All(ctx)
}

func (r *EventRepository) Delete(ctx context.Context, id int) error {
	return r.client.Event.DeleteOneID(id).Exec(ctx)
}

func (r *EventRepository) ListUpcoming(ctx context.Context, from, to time.Time) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Where(event.DateGTE(from), event.DateLTE(to)).
		Order(ent.Asc(event.FieldDate)).
		WithContact().
		All(ctx)
}
