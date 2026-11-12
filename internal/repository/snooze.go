package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/snooze"
)

// SnoozeRepository stores per-occurrence acknowledgements and snoozes. One row
// exists per (event, occasion, user); a re-post updates it in place so the
// action pages stay idempotent.
//
// occasion is always reduced to its calendar day in UTC before it is stored or
// matched: an occurrence is a date, never an instant, and the notification
// scheduler compares the same day-precision value.
type SnoozeRepository struct {
	client *ent.Client
}

// NewSnoozeRepository returns a repository backed by client.
func NewSnoozeRepository(client *ent.Client) *SnoozeRepository {
	return &SnoozeRepository{client: client}
}

// SnoozeOccasion is the day-precision key an action applies to.
func SnoozeOccasion(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// Upsert records that the given occurrence is snoozed (or handled) until the
// supplied instant, replacing an earlier value for the same key. userID 0 is
// the global sentinel used when a reminder is not addressed to one user.
func (r *SnoozeRepository) Upsert(ctx context.Context, eventID int, occasion time.Time, userID int, until time.Time) error {
	day := SnoozeOccasion(occasion)
	pred := func(q *ent.SnoozeQuery) *ent.SnoozeQuery {
		return q.Where(
			snooze.EventIDEQ(eventID),
			snooze.OccasionEQ(day),
			snooze.UserIDEQ(userID),
		)
	}

	existing, err := pred(r.client.Snooze.Query()).First(ctx)
	switch {
	case err == nil:
		return r.client.Snooze.UpdateOneID(existing.ID).
			SetSnoozedUntil(until).
			Exec(ctx)
	case !ent.IsNotFound(err):
		return err
	}

	_, err = r.client.Snooze.Create().
		SetEventID(eventID).
		SetOccasion(day).
		SetUserID(userID).
		SetSnoozedUntil(until).
		SetCreatedAt(time.Now()).
		Save(ctx)
	if ent.IsConstraintError(err) {
		// Lost a race against a concurrent writer: update the row that won.
		other, findErr := pred(r.client.Snooze.Query()).First(ctx)
		if findErr != nil {
			return findErr
		}
		return r.client.Snooze.UpdateOneID(other.ID).
			SetSnoozedUntil(until).
			Exec(ctx)
	}
	return err
}

// Delete removes the row for one occurrence, if it exists. Deleting a missing
// row is not an error, so callers can un-snooze unconditionally.
func (r *SnoozeRepository) Delete(ctx context.Context, eventID int, occasion time.Time, userID int) error {
	_, err := r.client.Snooze.Delete().
		Where(
			snooze.EventIDEQ(eventID),
			snooze.OccasionEQ(SnoozeOccasion(occasion)),
			snooze.UserIDEQ(userID),
		).
		Exec(ctx)
	return err
}

// Active reports whether the occurrence is still snoozed at now.
func (r *SnoozeRepository) Active(ctx context.Context, eventID int, occasion time.Time, userID int, now time.Time) (bool, error) {
	return r.client.Snooze.Query().
		Where(
			snooze.EventIDEQ(eventID),
			snooze.OccasionEQ(SnoozeOccasion(occasion)),
			snooze.UserIDEQ(userID),
			snooze.SnoozedUntilGT(now),
		).
		Exist(ctx)
}

// ListActive returns every snooze that has not expired at now, newest first.
func (r *SnoozeRepository) ListActive(ctx context.Context, now time.Time) ([]*ent.Snooze, error) {
	return r.client.Snooze.Query().
		Where(snooze.SnoozedUntilGT(now)).
		Order(ent.Desc(snooze.FieldSnoozedUntil)).
		All(ctx)
}
