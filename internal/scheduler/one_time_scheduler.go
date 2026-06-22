package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
)

// parseChannelTargets safely parses the channel_targets JSON field.
// Returns an empty slice if the field is empty or invalid.
func parseChannelTargets(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var channels []string
	if err := json.Unmarshal([]byte(raw), &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

const pollInterval = 30 * time.Second

type OneTimeNotificationScheduler struct {
	repo         *repository.OneTimeNotificationRepository
	deliveryRepo *repository.NotificationDeliveryRepository
	registry     *notifier.Registry
}

func NewOneTimeNotificationScheduler(repo *repository.OneTimeNotificationRepository, deliveryRepo *repository.NotificationDeliveryRepository, registry *notifier.Registry) *OneTimeNotificationScheduler {
	return &OneTimeNotificationScheduler{repo: repo, deliveryRepo: deliveryRepo, registry: registry}
}

func (s *OneTimeNotificationScheduler) Start(ctx context.Context) {
	slog.Info("one-time notification scheduler started", "source", "scheduler", "interval", pollInterval)

	// Run an immediate catch-up on startup for any notifications that were due while offline.
	s.processDue(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("one-time notification scheduler stopped", "source", "scheduler")
			return
		case <-ticker.C:
			s.processDue(ctx)
		}
	}
}

func (s *OneTimeNotificationScheduler) processDue(ctx context.Context) {
	due, err := s.repo.ListDue(ctx)
	if err != nil {
		slog.Error("one-time scheduler: list due notifications", "source", "scheduler", "error", err)
		return
	}

	if len(due) == 0 {
		return
	}

	slog.Info("one-time scheduler: processing due notifications", "source", "scheduler", "count", len(due))

	for _, n := range due {
		slog.Info("one-time scheduler: sending notification", "source", "scheduler", "id", n.ID)

		// Parse channel targets from JSON field.
		// For notifications created before the channel_targets migration,
		// this field may be empty — fall back to delivery records or all
		// configured notifiers.
		channels, err := parseChannelTargets(n.ChannelTargets)
		if err != nil {
			slog.Warn("one-time scheduler: channel_targets parse failed, falling back", "source", "scheduler", "id", n.ID, "error", err)
		}

		// Build lookup of delivery records by channel
		deliveryByChannel := make(map[string]*ent.NotificationDelivery)
		if n.Edges.Deliveries != nil {
			for _, d := range n.Edges.Deliveries {
				deliveryByChannel[d.Channel] = d
			}
		}

		// Fallback 1: derive channels from existing delivery records
		if len(channels) == 0 && len(deliveryByChannel) > 0 {
			for ch := range deliveryByChannel {
				channels = append(channels, ch)
			}
		}

		// Fallback 2: send to all configured notifiers (pre-migration behaviour)
		if len(channels) == 0 {
			channels = s.registry.ConfiguredNames()
		}

		if len(channels) == 0 {
			slog.Warn("one-time scheduler: no channels to send notification", "source", "scheduler", "id", n.ID)
			if err := s.repo.MarkFailed(ctx, n.ID); err != nil {
				slog.Error("one-time scheduler: mark failed", "source", "scheduler", "id", n.ID, "error", err)
			}
			continue
		}

		anySuccess := false
		for _, ch := range channels {
			if err := s.registry.Send(ctx, ch, "One-Time Notification", n.Message); err != nil {
				slog.Error("one-time scheduler: send failed", "source", "scheduler", "id", n.ID, "channel", ch, "error", err)
				if d, ok := deliveryByChannel[ch]; ok {
					if markErr := s.deliveryRepo.MarkFailed(ctx, d.ID, err.Error()); markErr != nil {
						slog.Error("one-time scheduler: mark delivery failed", "source", "scheduler", "id", n.ID, "channel", ch, "error", markErr)
					}
				}
			} else {
				anySuccess = true
				slog.Info("one-time scheduler: sent via channel", "source", "scheduler", "id", n.ID, "channel", ch)
				if d, ok := deliveryByChannel[ch]; ok {
					if err := s.deliveryRepo.MarkSent(ctx, d.ID); err != nil {
						slog.Error("one-time scheduler: mark delivery sent", "source", "scheduler", "id", n.ID, "channel", ch, "error", err)
					}
				}
			}
		}

		if anySuccess {
			if err := s.repo.MarkSent(ctx, n.ID); err != nil {
				slog.Error("one-time scheduler: mark sent", "source", "scheduler", "id", n.ID, "error", err)
			}
		} else {
			if err := s.repo.MarkFailed(ctx, n.ID); err != nil {
				slog.Error("one-time scheduler: mark failed", "source", "scheduler", "id", n.ID, "error", err)
			}
		}
	}
}
