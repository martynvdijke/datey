package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/notificationdelivery"
	"github.com/datey/datey/ent/onetimenotification"
)

type OneTimeNotificationRepository struct {
	client *ent.Client
}

func NewOneTimeNotificationRepository(client *ent.Client) *OneTimeNotificationRepository {
	return &OneTimeNotificationRepository{client: client}
}

func (r *OneTimeNotificationRepository) Create(ctx context.Context, message string, scheduledAt time.Time, channelTargets []string) (*ent.OneTimeNotification, error) {
	targetsJSON, _ := json.Marshal(channelTargets)

	n, err := r.client.OneTimeNotification.Create().
		SetMessage(message).
		SetScheduledAt(scheduledAt).
		SetCreatedAt(time.Now()).
		SetChannelTargets(string(targetsJSON)).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	// Create delivery tracking records for each channel
	for _, ch := range channelTargets {
		_, err := r.client.NotificationDelivery.Create().
			SetChannel(ch).
			SetStatus("pending").
			SetNotificationID(n.ID).
			Save(ctx)
		if err != nil {
			// Log but don't fail - delivery records are auxiliary
			return n, nil
		}
	}

	return n, nil
}

func (r *OneTimeNotificationRepository) List(ctx context.Context) ([]*ent.OneTimeNotification, error) {
	return r.client.OneTimeNotification.Query().
		WithDeliveries().
		Order(ent.Asc(onetimenotification.FieldScheduledAt)).
		All(ctx)
}

func (r *OneTimeNotificationRepository) ListDue(ctx context.Context) ([]*ent.OneTimeNotification, error) {
	return r.client.OneTimeNotification.Query().
		Where(
			onetimenotification.StatusEQ("pending"),
			onetimenotification.ScheduledAtLTE(time.Now()),
		).
		WithDeliveries().
		Order(ent.Asc(onetimenotification.FieldScheduledAt)).
		All(ctx)
}

func (r *OneTimeNotificationRepository) Get(ctx context.Context, id int) (*ent.OneTimeNotification, error) {
	return r.client.OneTimeNotification.Get(ctx, id)
}

func (r *OneTimeNotificationRepository) Delete(ctx context.Context, id int) error {
	// Delete delivery records first to avoid FK constraint
	_, err := r.client.NotificationDelivery.Delete().Where(
		notificationdelivery.HasNotificationWith(onetimenotification.IDEQ(id)),
	).Exec(ctx)
	if err != nil {
		return err
	}
	return r.client.OneTimeNotification.DeleteOneID(id).Exec(ctx)
}

func (r *OneTimeNotificationRepository) MarkSent(ctx context.Context, id int) error {
	return r.client.OneTimeNotification.UpdateOneID(id).
		SetStatus("sent").
		SetSentAt(time.Now()).
		Exec(ctx)
}

func (r *OneTimeNotificationRepository) MarkFailed(ctx context.Context, id int) error {
	return r.client.OneTimeNotification.UpdateOneID(id).
		SetStatus("failed").
		Exec(ctx)
}
