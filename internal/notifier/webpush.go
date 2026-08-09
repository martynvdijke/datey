package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
)

// WebPushNotifier delivers reminders as browser notifications via the Web Push
// protocol (RFC 8291) using VAPID-signed requests. All stored subscriptions
// receive each message; endpoints that return 404/410 (unsubscribed or
// expired) are pruned automatically.
type WebPushNotifier struct {
	cfg  *config.Config
	subs *repository.PushSubscriptionRepository
	// client is injectable for tests (webpush.HTTPClient is an interface).
	client webpush.HTTPClient
}

// NewWebPushNotifier creates a WebPushNotifier. The default HTTP client has a
// 10s timeout; the push payload is small, so this is generous.
func NewWebPushNotifier(cfg *config.Config, subs *repository.PushSubscriptionRepository) *WebPushNotifier {
	return &WebPushNotifier{
		cfg:    cfg,
		subs:   subs,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name implements Notifier.
func (n *WebPushNotifier) Name() string {
	return "webpush"
}

// IsConfigured implements Notifier. Web Push is only usable when enabled and
// both VAPID keys are present (env or generated on first enable).
func (n *WebPushNotifier) IsConfigured() bool {
	return n.cfg.PushEnabled &&
		n.cfg.PushVAPIDPublicKey != "" &&
		n.cfg.PushVAPIDPrivateKey != ""
}

// Send implements Notifier. It pushes to every stored subscription, pruning
// endpoints that report 404 (gone) or 410 (unsubscribed). Any other failure is
// returned so the caller can record it (e.g. a failed one-time delivery).
func (n *WebPushNotifier) Send(ctx context.Context, title, message string) error {
	if !n.IsConfigured() {
		return fmt.Errorf("webpush: not configured")
	}

	subscriptions, err := n.subs.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("webpush: list subscriptions: %w", err)
	}

	payload, err := json.Marshal(map[string]string{
		"title":   title,
		"message": message,
	})
	if err != nil {
		return fmt.Errorf("webpush: marshal payload: %w", err)
	}

	var firstErr error
	for _, sub := range subscriptions {
		resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.P256dh,
				Auth:   sub.Auth,
			},
		}, &webpush.Options{
			Subscriber:      "datey@localhost",
			VAPIDPublicKey:  n.cfg.PushVAPIDPublicKey,
			VAPIDPrivateKey: n.cfg.PushVAPIDPrivateKey,
			TTL:             60,
			HTTPClient:      n.client,
		})
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("webpush: send to %s: %w", sub.Endpoint, err)
			}
			continue
		}
		if resp != nil {
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
				// Subscription is stale — prune it so we don't keep failing.
				if delErr := n.subs.DeleteByEndpoint(ctx, sub.Endpoint); delErr != nil {
					if firstErr == nil {
						firstErr = fmt.Errorf("webpush: prune %s: %w", sub.Endpoint, delErr)
					}
				}
				_ = resp.Body.Close()
				continue
			}
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				if firstErr == nil {
					firstErr = fmt.Errorf("webpush: push service returned status %d for %s", resp.StatusCode, sub.Endpoint)
				}
			}
			_ = resp.Body.Close()
		}
	}

	return firstErr
}
