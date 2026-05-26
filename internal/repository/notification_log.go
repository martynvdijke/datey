package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/notificationlog"
)

type NotificationLogRepository struct {
	client *ent.Client
}

func NewNotificationLogRepository(client *ent.Client) *NotificationLogRepository {
	return &NotificationLogRepository{client: client}
}

func (r *NotificationLogRepository) Create(ctx context.Context, eventID int, channel, dateKey string, sentAt time.Time) (*ent.NotificationLog, error) {
	return r.client.NotificationLog.Create().
		SetChannel(channel).
		SetSentAt(sentAt).
		SetDateKey(dateKey).
		SetEventID(eventID).
		Save(ctx)
}

func (r *NotificationLogRepository) ExistsForDate(ctx context.Context, channel, dateKey string) (bool, error) {
	return r.client.NotificationLog.Query().
		Where(
			notificationlog.ChannelEQ(channel),
			notificationlog.DateKeyEQ(dateKey),
		).
		Exist(ctx)
}
