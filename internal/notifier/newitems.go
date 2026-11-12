package notifier

import (
	"context"
	"log/slog"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
)

// New-item channel types stored in user_notification_channel.
const (
	ChannelNewItems = "newitems"
	ChannelFCM      = "fcm"
)

// RealtimePublisher receives new-item notifications for in-app (SSE)
// delivery. internal/web's Broker satisfies it.
type RealtimePublisher interface {
	PublishNewItem(userIDs []int, kind, title, message, url string)
}

// NewItemDispatcher notifies every user except the actor when a person or
// event is created: realtime first, then Web Push and FCM where the user
// enabled them. All delivery happens off the request path.
type NewItemDispatcher struct {
	cfg       *config.Config
	client    *ent.Client
	webpush   *WebPushNotifier
	fcm       *FCMNotifier
	publisher RealtimePublisher
}

// NewNewItemDispatcher wires the dispatcher. Any dependency may be nil in
// tests or when a channel is not configured; the dispatcher degrades quietly.
func NewNewItemDispatcher(cfg *config.Config, client *ent.Client, webpush *WebPushNotifier, fcm *FCMNotifier, publisher RealtimePublisher) *NewItemDispatcher {
	return &NewItemDispatcher{
		cfg:       cfg,
		client:    client,
		webpush:   webpush,
		fcm:       fcm,
		publisher: publisher,
	}
}

// NotifyNewItem fans a new-item notification out to every eligible user. It
// never blocks the caller: the request context is detached (keeping values
// but not cancellation) and the work runs in its own goroutine with a bounded
// timeout. Every failure is logged, never returned.
func (d *NewItemDispatcher) NotifyNewItem(ctx context.Context, actorUserID int, kind, title, message, url string) {
	if d == nil || d.cfg == nil || !d.cfg.NotifyNewItems {
		return
	}
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	go func() {
		runCtx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		d.dispatch(runCtx, actorUserID, kind, title, message, url)
	}()
}

func (d *NewItemDispatcher) dispatch(ctx context.Context, actorUserID int, kind, title, message, url string) {
	if d.client == nil {
		return
	}

	users, err := d.client.User.Query().Select(user.FieldID).All(ctx)
	if err != nil {
		slog.Error("new items: list users", "source", "notifier", "error", err)
		return
	}

	rows, err := d.client.UserNotificationChannel.Query().WithUser().All(ctx)
	if err != nil {
		slog.Error("new items: list channels", "source", "notifier", "error", err)
		return
	}
	channels := make(map[int]map[string]*ent.UserNotificationChannel, len(rows))
	for _, ch := range rows {
		if ch.Edges.User == nil {
			continue
		}
		m := channels[ch.Edges.User.ID]
		if m == nil {
			m = make(map[string]*ent.UserNotificationChannel, 2)
			channels[ch.Edges.User.ID] = m
		}
		m[ch.ChannelType] = ch
	}

	eligible := make([]int, 0, len(users))
	for _, u := range users {
		if actorUserID != 0 && u.ID == actorUserID {
			continue
		}
		// Missing "newitems" row means the opt-out default: enabled.
		if ch, ok := channels[u.ID][ChannelNewItems]; ok && !ch.Enabled {
			continue
		}
		eligible = append(eligible, u.ID)
	}
	if len(eligible) == 0 {
		return
	}

	if d.publisher != nil {
		d.publisher.PublishNewItem(eligible, kind, title, message, url)
	}

	for _, userID := range eligible {
		m := channels[userID]
		if d.webpush != nil && channelEnabled(m, "webpush") && d.webpush.IsConfigured() {
			if err := d.webpush.SendToUser(ctx, userID, title, message); err != nil {
				slog.Error("new items: webpush send failed", "source", "notifier", "channel", "webpush", "user_id", userID, "error", err)
			}
		}
		if d.fcm != nil && channelEnabled(m, ChannelFCM) && d.fcm.IsConfigured() {
			if err := d.fcm.SendToUser(ctx, userID, title, message, url, kind); err != nil {
				slog.Error("new items: fcm send failed", "source", "notifier", "channel", "fcm", "user_id", userID, "error", err)
			}
		}
	}
}

// channelEnabled reports whether a per-user channel row exists and is enabled.
// A missing row means the channel was never turned on.
func channelEnabled(m map[string]*ent.UserNotificationChannel, channelType string) bool {
	ch, ok := m[channelType]
	return ok && ch != nil && ch.Enabled
}
