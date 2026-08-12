package repository

import (
	"context"
	"sort"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/contact"
	"github.com/datey/datey/ent/event"
	"github.com/datey/datey/ent/person"
	"github.com/datey/datey/internal/recurring"
)

type EventRepository struct {
	client *ent.Client
}

func NewEventRepository(client *ent.Client) *EventRepository {
	return &EventRepository{client: client}
}

func (r *EventRepository) Create(ctx context.Context, personID int, eventType string, date time.Time, description string) (*ent.Event, error) {
	return r.client.Event.Create().
		SetType(eventType).
		SetDate(date).
		SetDescription(description).
		SetCreatedAt(time.Now()).
		SetContactID(personID).
		Save(ctx)
}

func (r *EventRepository) CreateForPerson(ctx context.Context, personID int, eventType string, date time.Time, description string) (*ent.Event, error) {
	return r.client.Event.Create().
		SetType(eventType).
		SetDate(date).
		SetDescription(description).
		SetCreatedAt(time.Now()).
		SetPersonID(personID).
		Save(ctx)
}

func (r *EventRepository) Get(ctx context.Context, id int) (*ent.Event, error) {
	return r.client.Event.Get(ctx, id)
}

func (r *EventRepository) List(ctx context.Context) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Order(ent.Asc(event.FieldDate)).
		WithContact().
		WithPerson().
		All(ctx)
}

func (r *EventRepository) ListByContact(ctx context.Context, contactID int) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Where(event.HasContactWith(contact.IDEQ(contactID))).
		Order(ent.Asc(event.FieldDate)).
		All(ctx)
}

func (r *EventRepository) ListByPerson(ctx context.Context, personID int) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Where(event.HasPersonWith(person.IDEQ(personID))).
		Order(ent.Asc(event.FieldDate)).
		WithContact().
		WithPerson().
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
		WithPerson().
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
		WithPerson().
		All(ctx)
}

// EventOccurrence pairs a stored event with the concrete date it fires on.
// For annual event types (birthday, anniversary, wedding, holiday) Date is
// the occurrence date — the stored month/day applied to the year the range
// spans. For all other types it equals the stored event date.
type EventOccurrence struct {
	Event *ent.Event
	Date  time.Time
}

// ListUpcomingOccurrences returns every event with an occurrence inside the
// inclusive range [from, to]. Annual event types are expanded to their
// occurrence dates (so a historical birthday is reported on this year's
// date); one-off events are reported on their stored date. Results are
// sorted by occurrence date ascending.
func (r *EventRepository) ListUpcomingOccurrences(ctx context.Context, from, to time.Time) ([]EventOccurrence, error) {
	events, err := r.client.Event.Query().
		WithContact().
		WithPerson().
		All(ctx)
	if err != nil {
		return nil, err
	}

	var out []EventOccurrence
	for _, e := range events {
		if recurring.IsAnnualType(e.Type) {
			for _, occ := range recurring.OccurrencesIn(e.Date, from, to) {
				out = append(out, EventOccurrence{Event: e, Date: occ})
			}
			continue
		}
		if !e.Date.Before(from) && !e.Date.After(to) {
			out = append(out, EventOccurrence{Event: e, Date: e.Date})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Date.Before(out[j].Date)
	})
	return out, nil
}
