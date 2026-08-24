package repository

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/contact"
	"github.com/datey/datey/ent/event"
	"github.com/datey/datey/ent/group"
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

// CreateForGroup creates an event owned by a group rather than a single
// person or contact. Exactly one owner edge must be set; callers must not
// combine this with a person or contact ID.
func (r *EventRepository) CreateForGroup(ctx context.Context, groupID int, eventType string, date time.Time, description string) (*ent.Event, error) {
	return r.client.Event.Create().
		SetType(eventType).
		SetDate(date).
		SetDescription(description).
		SetCreatedAt(time.Now()).
		SetGroupID(groupID).
		Save(ctx)
}

func (r *EventRepository) Get(ctx context.Context, id int) (*ent.Event, error) {
	return r.client.Event.Get(ctx, id)
}

func (r *EventRepository) List(ctx context.Context) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Order(ent.Asc(event.FieldDate)).
		WithContact().
		WithGroup().
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

// FindByPersonAndType returns the person's event of the given type, or an
// ent.NotFoundError when none exists. When several exist, the earliest by
// date is returned.
func (r *EventRepository) FindByPersonAndType(ctx context.Context, personID int, eventType string) (*ent.Event, error) {
	return r.client.Event.Query().
		Where(event.HasPersonWith(person.IDEQ(personID)), event.TypeEQ(eventType)).
		Order(ent.Asc(event.FieldDate)).
		First(ctx)
}

// ListByGroup returns the events owned by a group, ordered by date.
func (r *EventRepository) ListByGroup(ctx context.Context, groupID int) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Where(event.HasGroupWith(group.IDEQ(groupID))).
		Order(ent.Asc(event.FieldDate)).
		WithContact().
		WithPerson().
		WithGroup().
		All(ctx)
}

func (r *EventRepository) Update(ctx context.Context, id int, eventType string, date time.Time, description string) (*ent.Event, error) {
	return r.client.Event.UpdateOneID(id).
		SetType(eventType).
		SetDate(date).
		SetDescription(description).
		Save(ctx)
}

// CreateForPersonWithCalendar creates an event with explicit calendar fields.
func (r *EventRepository) CreateForPersonWithCalendar(ctx context.Context, personID int, eventType string, date time.Time, description string, calendarSystem string, lunarMonth, lunarDay *int, lunarLeap bool) (*ent.Event, error) {
	if err := validateLunar(calendarSystem, lunarMonth, lunarDay); err != nil {
		return nil, err
	}
	cb := r.client.Event.Create().
		SetType(eventType).
		SetDate(date).
		SetDescription(description).
		SetCreatedAt(time.Now()).
		SetPersonID(personID).
		SetCalendarSystem(calendarSystem).
		SetLunarLeap(lunarLeap)
	if lunarMonth != nil {
		cb = cb.SetLunarMonth(*lunarMonth)
	}
	if lunarDay != nil {
		cb = cb.SetLunarDay(*lunarDay)
	}
	return cb.Save(ctx)
}

// UpdateWithCalendar updates an event with calendar fields.
func (r *EventRepository) UpdateWithCalendar(ctx context.Context, id int, eventType string, date time.Time, description string, calendarSystem string, lunarMonth, lunarDay *int, lunarLeap bool) (*ent.Event, error) {
	if err := validateLunar(calendarSystem, lunarMonth, lunarDay); err != nil {
		return nil, err
	}
	u := r.client.Event.UpdateOneID(id).
		SetType(eventType).
		SetDate(date).
		SetDescription(description).
		SetCalendarSystem(calendarSystem).
		SetLunarLeap(lunarLeap)
	if lunarMonth != nil {
		u = u.SetLunarMonth(*lunarMonth)
	} else {
		u = u.ClearLunarMonth()
	}
	if lunarDay != nil {
		u = u.SetLunarDay(*lunarDay)
	} else {
		u = u.ClearLunarDay()
	}
	return u.Save(ctx)
}

func (r *EventRepository) ListInRange(ctx context.Context, start, end time.Time) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Where(event.DateGTE(start), event.DateLTE(end)).
		Order(ent.Asc(event.FieldDate)).
		WithContact().
		WithGroup().
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
		WithGroup().
		WithPerson().
		All(ctx)
}

// EventOccurrence pairs a stored event with the concrete date it fires on.
// Every event recurs annually, so Date is the occurrence date — the stored
// month/day applied to the year the range spans.
type EventOccurrence struct {
	Event *ent.Event
	Date  time.Time
}

// validateLunar checks that lunar events carry required fields.
func validateLunar(calendarSystem string, lunarMonth, lunarDay *int) error {
	if calendarSystem == "lunar" {
		if lunarMonth == nil || lunarDay == nil {
			return fmt.Errorf("lunar event requires lunar_month and lunar_day")
		}
		if *lunarMonth < 1 || *lunarMonth > 12 {
			return fmt.Errorf("lunar_month out of range")
		}
		if *lunarDay < 1 || *lunarDay > 30 {
			return fmt.Errorf("lunar_day out of range")
		}
	}
	return nil
}

// ListUpcomingOccurrences returns every event with an occurrence inside the
// inclusive range [from, to]. All events recur annually: for gregorian events
// the stored month/day is expanded; for lunar events the lunar month/day is
// converted to Gregorian per target year before window comparison.
// Results are sorted by occurrence date ascending.
func (r *EventRepository) ListUpcomingOccurrences(ctx context.Context, from, to time.Time) ([]EventOccurrence, error) {
	events, err := r.client.Event.Query().
		WithContact().
		WithGroup().
		WithPerson().
		All(ctx)
	if err != nil {
		return nil, err
	}

	var out []EventOccurrence
	for _, e := range events {
		for _, occ := range recurring.OccurrencesInForEvent(e, from, to) {
			out = append(out, EventOccurrence{Event: e, Date: occ})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Date.Before(out[j].Date)
	})
	return out, nil
}

// NextBirthdayOccurrence returns the earliest birthday occurrence at or
// after from, or nil when no birthday event exists. Annual expansion rules
// match ListUpcomingOccurrences, lunar-aware.
func (r *EventRepository) NextBirthdayOccurrence(ctx context.Context, from time.Time) (*EventOccurrence, error) {
	events, err := r.client.Event.Query().
		Where(event.TypeEQ("birthday")).
		WithContact().
		WithGroup().
		WithPerson().
		All(ctx)
	if err != nil {
		return nil, err
	}

	to := from.AddDate(0, 0, 366)
	var best *EventOccurrence
	for _, e := range events {
		for _, occ := range recurring.OccurrencesInForEvent(e, from, to) {
			if best != nil && !occ.Before(best.Date) {
				continue
			}
			best = &EventOccurrence{Event: e, Date: occ}
		}
	}
	return best, nil
}
