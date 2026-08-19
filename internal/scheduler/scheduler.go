package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/carddav"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/settings"
)

// catchUpGrace is the epsilon that separates a normal restart (shortly after
// the scheduled hour, no catch-up needed) from genuine downtime (catch-up
// runs). It prevents a restart a few minutes past the scheduled hour from
// double-firing the same day's pass.
const catchUpGrace = 30 * time.Minute

// catchUpMinGap is the minimum gap between the last recorded run and now that
// triggers a catch-up pass: one day plus the grace period.
const catchUpMinGap = 24*time.Hour + catchUpGrace

type Scheduler struct {
	cfg      *config.Config
	client   *ent.Client
	registry *notifier.Registry
	events   *repository.EventRepository
	notifLog *repository.NotificationLogRepository
	settings *settings.Store
}

func New(cfg *config.Config, client *ent.Client, registry *notifier.Registry, settingsStore *settings.Store) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		client:   client,
		registry: registry,
		events:   repository.NewEventRepository(client),
		notifLog: repository.NewNotificationLogRepository(client),
		settings: settingsStore,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	slog.Info("scheduler started", "source", "scheduler", "hour", s.cfg.SchedulerHour)

	s.catchUpMissed(ctx)

	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), s.cfg.SchedulerHour, 0, 0, 0, now.Location())
	if now.After(next) {
		next = next.Add(24 * time.Hour)
	}

	initialDelay := time.Until(next)
	slog.Debug("scheduler first run", "source", "scheduler", "delay", initialDelay)

	timer := time.NewTimer(initialDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler stopped", "source", "scheduler")
			return
		case <-timer.C:
			s.processReminders(ctx, false)
			s.persistRun(ctx)
			s.runBackup(ctx)
			s.runWeeklyBackup(ctx)
			s.runCarddavSync(ctx)
			timer.Reset(24 * time.Hour)
		}
	}
}

// runCarddavSync triggers a daily CardDAV sync when enabled and the last sync
// is older than 24 hours. The rate limit (tracked via carddav_last_sync) keeps
// an address book from being hammered on every scheduler tick; the manual
// "Sync now" button in Settings bypasses it.
func (s *Scheduler) runCarddavSync(ctx context.Context) {
	if !s.cfg.CarddavEnabled || s.cfg.CarddavURL == "" {
		return
	}

	lastSync, err := s.settings.CarddavLastSync(ctx)
	if err != nil {
		slog.Error("scheduler: read carddav last sync", "source", "scheduler", "error", err)
		return
	}
	if lastSync != nil && time.Since(*lastSync) < 24*time.Hour {
		slog.Debug("scheduler: carddav sync not due", "source", "scheduler", "last_sync", lastSync.Format(time.RFC3339))
		return
	}

	slog.Info("scheduler: running carddav sync", "source", "scheduler")
	syncer := carddav.NewSyncer(s.cfg, s.client, s.settings)
	if _, err := syncer.Sync(ctx, carddav.SyncFull, false); err != nil {
		slog.Error("scheduler: carddav sync failed", "source", "scheduler", "error", err)
	}
}

// catchUpMissed runs a startup catch-up pass when the gap since the last
// successful reminder pass exceeds one day plus a grace period. It notifies
// occurrences whose dates fell inside the reminder window during the downtime
// and were never notified, using missed-date phrasing. Skipped entirely when
// SCHEDULER_CATCHUP is false.
func (s *Scheduler) catchUpMissed(ctx context.Context) {
	if !s.cfg.SchedulerCatchup {
		slog.Info("scheduler catch-up disabled", "source", "scheduler")
		return
	}

	lastRun, err := s.settings.LastSchedulerRun(ctx)
	if err != nil {
		slog.Error("scheduler: read last run", "source", "scheduler", "error", err)
		return
	}

	now := time.Now()
	if lastRun != nil && now.Sub(*lastRun) <= catchUpMinGap {
		slog.Debug("scheduler: no catch-up needed", "source", "scheduler", "last_run", lastRun.Format(time.RFC3339))
		return
	}

	from := now.AddDate(0, 0, -s.cfg.ReminderDays)
	if lastRun != nil {
		from = lastRun.AddDate(0, 0, -s.cfg.ReminderDays)
	}
	slog.Info("scheduler: catching up missed reminders", "source", "scheduler", "from", from.Format(time.RFC3339))

	s.processReminders(ctx, true)
	s.persistRun(ctx)
}

// persistRun records the current time as the last successful reminder pass in
// app_config, so a subsequent restart can compute the downtime gap.
func (s *Scheduler) persistRun(ctx context.Context) {
	if err := s.settings.SetLastSchedulerRun(ctx, time.Now()); err != nil {
		slog.Error("scheduler: persist last run", "source", "scheduler", "error", err)
	}
}

func (s *Scheduler) processReminders(ctx context.Context, catchUp bool) {
	now := time.Now()

	// The reminder window starts at the beginning of today (midnight UTC,
	// matching how event dates and annual occurrences are stored). Starting
	// at time.Now() would exclude events dated today — their midnight-UTC
	// date is earlier than the pass time — so they would never be reminded.
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	end := from.AddDate(0, 0, s.cfg.ReminderDays)

	if catchUp {
		// The catch-up pass looks backwards: occurrences with dates inside
		// [lastRun - ReminderDays, now] could have been reminded during the
		// downtime window. Occurrences after now are covered by the next
		// timed pass, so the catch-up window is capped at now.
		end = now
		from = now.AddDate(0, 0, -s.cfg.ReminderDays)
		if lastRun, err := s.settings.LastSchedulerRun(ctx); err == nil && lastRun != nil {
			from = lastRun.AddDate(0, 0, -s.cfg.ReminderDays)
		}
	}

	occurrences, err := s.events.ListUpcomingOccurrences(ctx, from, end)
	if err != nil {
		slog.Error("scheduler: list upcoming events", "source", "scheduler", "error", err)
		return
	}

	slog.Info("processing reminders", "source", "scheduler", "event_count", len(occurrences), "catch_up", catchUp)

	for _, occ := range occurrences {
		event := occ.Event

		// Per-person opt-out: birthday-type occurrences are skipped for
		// persons who disabled birthday notifications. Other event types
		// are unaffected.
		if event.Type == "birthday" {
			if p := event.Edges.Person; p != nil && !p.NotifyBirthdays {
				slog.Debug("scheduler: birthday notifications disabled", "source", "scheduler", "person", p.ID, "event", event.ID)
				continue
			}
		}

		eventKey := fmt.Sprintf("%d-%s", event.ID, occ.Date.Format("2006-01-02"))

		for _, name := range []string{"email", "gotify", "telegram", "ntfy", "webhook", "webpush"} {
			if !s.registry.IsConfigured(name) {
				continue
			}

			dateKey := fmt.Sprintf("%s-%s", name, eventKey)
			exists, err := s.notifLog.ExistsForDate(ctx, name, dateKey)
			if err != nil {
				slog.Error("scheduler: check notification log", "source", "scheduler", "error", err)
				continue
			}
			if exists {
				slog.Debug("scheduler: notification already sent", "source", "scheduler", "channel", name, "event", event.ID)
				continue
			}

			contactName := ""
			if contact := event.Edges.Contact; contact != nil {
				contactName = contact.Name
			} else if p := event.Edges.Person; p != nil {
				contactName = p.Name
			}

			title := fmt.Sprintf("Reminder: %s - %s", contactName, event.Type)

			days := daysBetween(now, occ.Date)
			var message string
			if catchUp && occ.Date.Before(now) {
				when := fmt.Sprintf("%d days ago", -days)
				switch days {
				case 0:
					when = "today"
				case -1:
					when = "yesterday"
				}
				message = fmt.Sprintf(
					"Missed reminder: %s for %s was %s (%s)",
					event.Type, contactName, occ.Date.Format("January 2"), when,
				)
			} else {
				when := fmt.Sprintf("%d days away", days)
				if days <= 0 {
					when = "today"
				} else if days == 1 {
					when = "tomorrow"
				}
				message = fmt.Sprintf(
					"Upcoming %s for %s on %s (%s)",
					event.Type, contactName, occ.Date.Format("January 2"), when,
				)
			}

			s.registry.SendAll(ctx, title, message)

			_, err = s.notifLog.Create(ctx, event.ID, name, dateKey, time.Now())
			if err != nil {
				slog.Error("scheduler: log notification", "source", "scheduler", "error", err)
			} else {
				slog.Info("scheduler: notification logged", "source", "scheduler", "channel", name, "event", event.ID)
			}
		}
	}
}

// daysBetween returns the calendar-day difference between now and t: how many
// days t is after now (negative when before). Both are truncated to their UTC
// date, so an occurrence later today reads 0 ("today") and one tomorrow reads
// 1, regardless of the pass time of day.
func daysBetween(now, t time.Time) int {
	nowDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	tDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return int(tDay.Sub(nowDay).Hours() / 24)
}

func (s *Scheduler) runBackup(ctx context.Context) {
	dbPath := s.cfg.DataDir + "/datey.db"
	slog.Info("running database backup", "source", "scheduler", "path", dbPath)

	if err := db.Backup(dbPath, s.cfg.BackupDir, s.cfg.BackupRetentionDays); err != nil {
		slog.Error("scheduler: backup failed", "source", "scheduler", "error", err)
		return
	}

	slog.Info("database backup completed", "source", "scheduler", "dir", s.cfg.BackupDir)
}

// runWeeklyBackup creates a weekly backup when today is the configured
// WEEKLY_BACKUP_DAY (0=Sunday..6=Saturday). Weekly backups use their own
// retention, so they survive the shorter daily retention window.
func (s *Scheduler) runWeeklyBackup(ctx context.Context) {
	if int(time.Now().Weekday()) != s.cfg.WeeklyBackupDay {
		return
	}

	dbPath := s.cfg.DataDir + "/datey.db"
	slog.Info("running weekly database backup", "source", "scheduler", "path", dbPath)

	if err := db.BackupWeekly(dbPath, s.cfg.BackupDir, s.cfg.WeeklyBackupRetentionWeeks); err != nil {
		slog.Error("scheduler: weekly backup failed", "source", "scheduler", "error", err)
		return
	}

	slog.Info("weekly database backup completed", "source", "scheduler", "dir", s.cfg.BackupDir)
}
