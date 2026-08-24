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
	return r.CreateForUser(ctx, eventID, channel, dateKey, 0, sentAt)
}

func (r *NotificationLogRepository) CreateForUser(ctx context.Context, eventID int, channel, dateKey string, userID int, sentAt time.Time) (*ent.NotificationLog, error) {
	return r.client.NotificationLog.Create().
		SetChannel(channel).
		SetSentAt(sentAt).
		SetDateKey(dateKey).
		SetUserID(userID).
		SetEventID(eventID).
		Save(ctx)
}

func (r *NotificationLogRepository) ExistsForDate(ctx context.Context, channel, dateKey string) (bool, error) {
	return r.ExistsForUser(ctx, channel, dateKey, 0)
}

func (r *NotificationLogRepository) ExistsForUser(ctx context.Context, channel, dateKey string, userID int) (bool, error) {
	return r.client.NotificationLog.Query().
		Where(
			notificationlog.ChannelEQ(channel),
			notificationlog.DateKeyEQ(dateKey),
			notificationlog.UserIDEQ(userID),
		).
		Exist(ctx)
}
