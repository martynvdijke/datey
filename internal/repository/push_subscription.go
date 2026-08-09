package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/pushsubscription"
	"github.com/datey/datey/ent/user"
)

// PushSubscriptionRepository manages stored browser Web Push subscriptions.
type PushSubscriptionRepository struct {
	client *ent.Client
}

func NewPushSubscriptionRepository(client *ent.Client) *PushSubscriptionRepository {
	return &PushSubscriptionRepository{client: client}
}

// Upsert replaces any existing subscription for the same endpoint (browsers
// re-issue the same endpoint URL on re-subscribe) or creates a new one owned
// by the given user.
func (r *PushSubscriptionRepository) Upsert(ctx context.Context, userID int, endpoint, p256dh, auth string) (*ent.PushSubscription, error) {
	now := time.Now()
	existing, err := r.client.PushSubscription.Query().
		Where(pushsubscription.EndpointEQ(endpoint)).
		First(ctx)
	if ent.IsNotFound(err) {
		return r.client.PushSubscription.Create().
			SetEndpoint(endpoint).
			SetP256dh(p256dh).
			SetAuth(auth).
			SetCreatedAt(now).
			SetUserID(userID).
			Save(ctx)
	}
	if err != nil {
		return nil, err
	}
	return r.client.PushSubscription.UpdateOneID(existing.ID).
		SetP256dh(p256dh).
		SetAuth(auth).
		SetUserID(userID).
		Save(ctx)
}

// ListAll returns every stored subscription.
func (r *PushSubscriptionRepository) ListAll(ctx context.Context) ([]*ent.PushSubscription, error) {
	return r.client.PushSubscription.Query().
		Order(ent.Asc(pushsubscription.FieldCreatedAt)).
		All(ctx)
}

// ListByUser returns the subscriptions owned by a single user.
func (r *PushSubscriptionRepository) ListByUser(ctx context.Context, userID int) ([]*ent.PushSubscription, error) {
	return r.client.PushSubscription.Query().
		Where(pushsubscription.HasUserWith(user.IDEQ(userID))).
		Order(ent.Asc(pushsubscription.FieldCreatedAt)).
		All(ctx)
}

// DeleteByEndpoint removes a subscription (used when a push provider reports
// the endpoint as gone — HTTP 404/410).
func (r *PushSubscriptionRepository) DeleteByEndpoint(ctx context.Context, endpoint string) error {
	_, err := r.client.PushSubscription.Delete().
		Where(pushsubscription.EndpointEQ(endpoint)).
		Exec(ctx)
	return err
}

// Count returns the total number of stored subscriptions.
func (r *PushSubscriptionRepository) Count(ctx context.Context) (int, error) {
	return r.client.PushSubscription.Query().Count(ctx)
}
