package repository

import (
	"context"
	"strings"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/ent/usernotificationchannel"
)

type UserNotificationChannelRepository struct {
	client *ent.Client
}

func NewUserNotificationChannelRepository(client *ent.Client) *UserNotificationChannelRepository {
	return &UserNotificationChannelRepository{client: client}
}

func (r *UserNotificationChannelRepository) Upsert(ctx context.Context, userID int, channelType, target string, enabled bool) (*ent.UserNotificationChannel, error) {
	channels, err := r.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, ch := range channels {
		if ch.ChannelType == channelType {
			return r.client.UserNotificationChannel.UpdateOneID(ch.ID).SetTarget(target).SetEnabled(enabled).Save(ctx)
		}
	}
	return r.client.UserNotificationChannel.Create().SetUserID(userID).SetChannelType(channelType).SetTarget(target).SetEnabled(enabled).Save(ctx)
}

func (r *UserNotificationChannelRepository) ListByUser(ctx context.Context, userID int) ([]*ent.UserNotificationChannel, error) {
	return r.client.UserNotificationChannel.Query().Where(usernotificationchannel.HasUserWith(user.IDEQ(userID))).All(ctx)
}

func (r *UserNotificationChannelRepository) MapByUser(ctx context.Context, userID int) (map[string]*ent.UserNotificationChannel, error) {
	list, err := r.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*ent.UserNotificationChannel, len(list))
	for _, ch := range list {
		m[ch.ChannelType] = ch
	}
	return m, nil
}

func (r *UserNotificationChannelRepository) Delete(ctx context.Context, id int) error {
	return r.client.UserNotificationChannel.DeleteOneID(id).Exec(ctx)
}

func ValidateTarget(channelType, target string) error {
	if strings.TrimSpace(target) == "" {
		return nil
	}
	if channelType == "email" && !strings.Contains(target, "@") {
		return invalidTargetError("email must contain @")
	}
	return nil
}

type invalidTargetError string

func (e invalidTargetError) Error() string { return string(e) }
