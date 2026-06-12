package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/notificationdelivery"
	"github.com/datey/datey/ent/onetimenotification"
)

type NotificationDeliveryRepository struct {
	client *ent.Client
}

func NewNotificationDeliveryRepository(client *ent.Client) *NotificationDeliveryRepository {
	return &NotificationDeliveryRepository{client: client}
}

func (r *NotificationDeliveryRepository) Create(ctx context.Context, notificationID int, channel string) (*ent.NotificationDelivery, error) {
	return r.client.NotificationDelivery.Create().
		SetChannel(channel).
		SetStatus("pending").
		SetNotificationID(notificationID).
		Save(ctx)
}

func (r *NotificationDeliveryRepository) MarkSent(ctx context.Context, id int) error {
	return r.client.NotificationDelivery.UpdateOneID(id).
		SetStatus("sent").
		SetSentAt(time.Now()).
		Exec(ctx)
}

func (r *NotificationDeliveryRepository) MarkFailed(ctx context.Context, id int, errMsg string) error {
	return r.client.NotificationDelivery.UpdateOneID(id).
		SetStatus("failed").
		SetErrorMessage(errMsg).
		SetSentAt(time.Now()).
		Exec(ctx)
}

func (r *NotificationDeliveryRepository) ListByNotification(ctx context.Context, notificationID int) ([]*ent.NotificationDelivery, error) {
	return r.client.NotificationDelivery.Query().
		Where(notificationdelivery.HasNotificationWith(onetimenotification.IDEQ(notificationID))).
		All(ctx)
}
