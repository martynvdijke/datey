package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
)

const pollInterval = 30 * time.Second

type OneTimeNotificationScheduler struct {
	repo     *repository.OneTimeNotificationRepository
	registry *notifier.Registry
}

func NewOneTimeNotificationScheduler(repo *repository.OneTimeNotificationRepository, registry *notifier.Registry) *OneTimeNotificationScheduler {
	return &OneTimeNotificationScheduler{repo: repo, registry: registry}
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

		s.registry.SendAll(ctx, "One-Time Notification", n.Message)

		err := s.repo.MarkSent(ctx, n.ID)
		if err != nil {
			slog.Error("one-time scheduler: mark sent", "source", "scheduler", "id", n.ID, "error", err)
		}

		slog.Info("one-time scheduler: notification sent", "source", "scheduler", "id", n.ID)
	}
}
