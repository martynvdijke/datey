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

		// Parse channel targets from JSON field
		var channels []string
		if err := json.Unmarshal([]byte(n.ChannelTargets), &channels); err != nil {
			slog.Error("one-time scheduler: parse channel targets", "source", "scheduler", "id", n.ID, "error", err)
			s.repo.MarkFailed(ctx, n.ID)
			continue
		}

		// Build lookup of delivery records by channel
		deliveryByChannel := make(map[string]*ent.NotificationDelivery)
		if n.Edges.Deliveries != nil {
			for _, d := range n.Edges.Deliveries {
				deliveryByChannel[d.Channel] = d
			}
		}

		anySuccess := false
		for _, ch := range channels {
			if err := s.registry.Send(ctx, ch, "One-Time Notification", n.Message); err != nil {
				slog.Error("one-time scheduler: send failed", "source", "scheduler", "id", n.ID, "channel", ch, "error", err)
				if d, ok := deliveryByChannel[ch]; ok {
					s.deliveryRepo.MarkFailed(ctx, d.ID, err.Error())
				}
			} else {
				anySuccess = true
				slog.Info("one-time scheduler: sent via channel", "source", "scheduler", "id", n.ID, "channel", ch)
				if d, ok := deliveryByChannel[ch]; ok {
					s.deliveryRepo.MarkSent(ctx, d.ID)
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
