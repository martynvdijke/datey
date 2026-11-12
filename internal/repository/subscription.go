package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/event"
	"github.com/datey/datey/ent/subscription"
)

// SubscriptionRepository persists remote ICS calendar subscriptions.
type SubscriptionRepository struct {
	client *ent.Client
}

func NewSubscriptionRepository(client *ent.Client) *SubscriptionRepository {
	return &SubscriptionRepository{client: client}
}

// List returns every subscription ordered by name.
func (r *SubscriptionRepository) List(ctx context.Context) ([]*ent.Subscription, error) {
	return r.client.Subscription.Query().
		Order(ent.Asc(subscription.FieldName), ent.Asc(subscription.FieldID)).
		All(ctx)
}

// ListEnabled returns only subscriptions eligible for polling, ordered by name.
func (r *SubscriptionRepository) ListEnabled(ctx context.Context) ([]*ent.Subscription, error) {
	return r.client.Subscription.Query().
		Where(subscription.EnabledEQ(true)).
		Order(ent.Asc(subscription.FieldName), ent.Asc(subscription.FieldID)).
		All(ctx)
}

// Get returns one subscription by ID.
func (r *SubscriptionRepository) Get(ctx context.Context, id int) (*ent.Subscription, error) {
	return r.client.Subscription.Get(ctx, id)
}

// Create inserts a subscription.
func (r *SubscriptionRepository) Create(ctx context.Context, name, url string, enabled bool) (*ent.Subscription, error) {
	return r.client.Subscription.Create().
		SetName(name).
		SetURL(url).
		SetEnabled(enabled).
		Save(ctx)
}

// Update changes the user-editable fields (name, URL, enabled).
func (r *SubscriptionRepository) Update(ctx context.Context, id int, name, url string, enabled bool) (*ent.Subscription, error) {
	return r.client.Subscription.UpdateOneID(id).
		SetName(name).
		SetURL(url).
		SetEnabled(enabled).
		Save(ctx)
}

// UpdateFetchStatus records the outcome of one fetch attempt. The attempt
// timestamp is always stored; a success also advances last_success_at and
// clears last_error, while a failure keeps the previous last_success_at so the
// UI can show how stale the last good state is.
func (r *SubscriptionRepository) UpdateFetchStatus(ctx context.Context, id int, attemptedAt time.Time, success bool, errMsg string) error {
	u := r.client.Subscription.UpdateOneID(id).
		SetLastAttemptAt(attemptedAt).
		SetLastError(errMsg)
	if success {
		u = u.SetLastSuccessAt(attemptedAt)
	}
	return u.Exec(ctx)
}

// Delete removes a subscription. Its events are detached first
// (subscription_id cleared) rather than deleted, because ent does not emit
// ON DELETE SET NULL and losing user-visible dates on unsubscribe is not
// acceptable.
func (r *SubscriptionRepository) Delete(ctx context.Context, id int) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin subscription delete: %w", err)
	}
	if _, err := tx.Event.Update().
		Where(event.SubscriptionIDEQ(id)).
		ClearSubscriptionID().
		Save(ctx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("detach subscription events: %w", err)
	}
	if err := tx.Subscription.DeleteOneID(id).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete subscription: %w", err)
	}
	return tx.Commit()
}

// EventsBySubscription returns the events owned by one subscription, ordered by
// date, with their owner edges preloaded for display.
func (r *SubscriptionRepository) EventsBySubscription(ctx context.Context, subscriptionID int) ([]*ent.Event, error) {
	return r.client.Event.Query().
		Where(event.SubscriptionIDEQ(subscriptionID)).
		Order(ent.Asc(event.FieldDate), ent.Asc(event.FieldID)).
		WithPerson().
		WithGroup().
		WithContact().
		All(ctx)
}

// EventBySubscriptionUID finds one subscription-owned event by its upstream
// UID. It returns ent.NotFoundError when the UID is not present.
func (r *SubscriptionRepository) EventBySubscriptionUID(ctx context.Context, subscriptionID int, uid string) (*ent.Event, error) {
	return r.client.Event.Query().
		Where(event.SubscriptionIDEQ(subscriptionID), event.ExternalUIDEQ(uid)).
		Only(ctx)
}

// CreateForSubscription creates an event owned by a subscription. Subscription
// events have no person, group or contact owner; ownership is expressed solely
// by subscription_id plus the upstream UID.
func (r *EventRepository) CreateForSubscription(ctx context.Context, subscriptionID int, uid, eventType string, date time.Time, description string) (*ent.Event, error) {
	return r.client.Event.Create().
		SetType(eventType).
		SetDate(date).
		SetDescription(description).
		SetCreatedAt(time.Now()).
		SetFrequency("annual").
		SetInterval(1).
		SetSubscriptionID(subscriptionID).
		SetExternalUID(uid).
		Save(ctx)
}

// UpdateForSubscription updates the feed-derived fields of a subscription
// event. Ownership (subscription_id, external_uid) is left untouched.
func (r *EventRepository) UpdateForSubscription(ctx context.Context, id int, eventType string, date time.Time, description string) (*ent.Event, error) {
	return r.client.Event.UpdateOneID(id).
		SetType(eventType).
		SetDate(date).
		SetDescription(description).
		Save(ctx)
}
