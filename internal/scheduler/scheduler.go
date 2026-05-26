package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
)

type Scheduler struct {
	cfg      *config.Config
	client   *ent.Client
	registry *notifier.Registry
	events   *repository.EventRepository
	notifLog *repository.NotificationLogRepository
}

func New(cfg *config.Config, client *ent.Client, registry *notifier.Registry) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		client:   client,
		registry: registry,
		events:   repository.NewEventRepository(client),
		notifLog: repository.NewNotificationLogRepository(client),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	slog.Info("scheduler started", "hour", s.cfg.SchedulerHour)

	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), s.cfg.SchedulerHour, 0, 0, 0, now.Location())
	if now.After(next) {
		next = next.Add(24 * time.Hour)
	}

	initialDelay := time.Until(next)
	slog.Debug("scheduler first run", "delay", initialDelay)

	timer := time.NewTimer(initialDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler stopped")
			return
		case <-timer.C:
			s.processReminders(ctx)
			timer.Reset(24 * time.Hour)
		}
	}
}

func (s *Scheduler) processReminders(ctx context.Context) {
	slog.Info("processing reminders")

	now := time.Now()
	end := now.AddDate(0, 0, s.cfg.ReminderDays)

	upcomingEvents, err := s.events.ListUpcoming(ctx, now, end)
	if err != nil {
		slog.Error("scheduler: list upcoming events", "error", err)
		return
	}

	for _, event := range upcomingEvents {
		eventKey := fmt.Sprintf("%d-%s", event.ID, event.Date.Format("2006-01-02"))

		for _, name := range []string{"email", "gotify", "telegram"} {
			if !s.registry.IsConfigured(name) {
				continue
			}

			dateKey := fmt.Sprintf("%s-%s", name, eventKey)
			exists, err := s.notifLog.ExistsForDate(ctx, name, dateKey)
			if err != nil {
				slog.Error("scheduler: check notification log", "error", err)
				continue
			}
			if exists {
				slog.Debug("scheduler: notification already sent", "channel", name, "event", event.ID)
				continue
			}

			contactName := ""
			if contact := event.Edges.Contact; contact != nil {
				contactName = contact.Name
			}

			title := fmt.Sprintf("Reminder: %s - %s", contactName, event.Type)
			message := fmt.Sprintf(
				"Upcoming %s for %s on %s (%d days away)",
				event.Type, contactName, event.Date.Format("January 2"), int(event.Date.Sub(now).Hours()/24),
			)

			s.registry.SendAll(ctx, title, message)

			_, err = s.notifLog.Create(ctx, event.ID, name, dateKey, time.Now())
			if err != nil {
				slog.Error("scheduler: log notification", "error", err)
			}
		}
	}
}
