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
	users    *repository.UserRepository
	channels *repository.UserNotificationChannelRepository
}

func New(cfg *config.Config, client *ent.Client, registry *notifier.Registry, settingsStore *settings.Store) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		client:   client,
		registry: registry,
		events:   repository.NewEventRepository(client),
		notifLog: repository.NewNotificationLogRepository(client),
		settings: settingsStore,
		users:    repository.NewUserRepository(client),
		channels: repository.NewUserNotificationChannelRepository(client),
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
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	end := from.AddDate(0, 0, s.cfg.ReminderDays)
	if catchUp {
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
	users, err := s.users.List(ctx)
	if err != nil {
		slog.Error("scheduler: list users", "source", "scheduler", "error", err)
		return
	}
	if len(users) == 0 {
		// Legacy fallback: no users, deliver globally once per event/channel
		for _, occ := range occurrences {
			event := occ.Event
			if event.Type == "birthday" {
				if p := event.Edges.Person; p != nil && !p.NotifyBirthdays {
					continue
				}
			}
			eventKey := fmt.Sprintf("%d-%s", event.ID, occ.Date.Format("2006-01-02"))
			for _, name := range []string{"email", "gotify", "telegram", "ntfy", "webhook", "webpush", "discord", "slack", "matrix"} {
				if !s.registry.IsConfigured(name) {
					continue
				}
				dateKey := fmt.Sprintf("%s-%s", name, eventKey)
				exists, err := s.notifLog.ExistsForDate(ctx, name, dateKey)
				if err != nil {
					continue
				}
				if exists {
					continue
				}
				contactName := ""
				if contact := event.Edges.Contact; contact != nil {
					contactName = contact.Name
				} else if p := event.Edges.Person; p != nil {
					contactName = p.Name
				} else if g := event.Edges.Group; g != nil {
					contactName = g.Name + " (group)"
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
					message = fmt.Sprintf("Missed reminder: %s for %s was %s (%s)", event.Type, contactName, occ.Date.Format("January 2"), when)
				} else {
					when := fmt.Sprintf("%d days away", days)
					if days <= 0 {
						when = "today"
					} else if days == 1 {
						when = "tomorrow"
					}
					message = fmt.Sprintf("Upcoming %s for %s on %s (%s)", event.Type, contactName, occ.Date.Format("January 2"), when)
				}
				_ = s.registry.Send(ctx, name, title, message)
				_, _ = s.notifLog.Create(ctx, event.ID, name, dateKey, time.Now())
			}
		}
		return
	}
	for _, occ := range occurrences {
		event := occ.Event
		if event.Type == "birthday" {
			if p := event.Edges.Person; p != nil && !p.NotifyBirthdays {
				slog.Debug("scheduler: birthday notifications disabled", "source", "scheduler", "person", p.ID, "event", event.ID)
				continue
			}
		}
		eventKey := fmt.Sprintf("%d-%s", event.ID, occ.Date.Format("2006-01-02"))
		for _, u := range users {
			if !s.userInScope(u, event) {
				continue
			}
			for _, name := range []string{"email", "gotify", "telegram", "ntfy", "webhook", "webpush", "discord", "slack", "matrix"} {
				if !s.registry.IsConfigured(name) {
					continue
				}
				target, hasTarget := s.resolveTarget(ctx, u.ID, name)
				if !hasTarget && !s.hasGlobalFallback(name) {
					continue
				}
				// If user has no per-user config and global fallback exists, hasTarget is false but we still deliver via global.
				// If user has per-user channel disabled, skip.
				if hasTarget {
					m, _ := s.channels.MapByUser(ctx, u.ID)
					if ch, ok := m[name]; ok && !ch.Enabled {
						continue
					}
				}
				var dateKey string
				var dedupUserID int
				if hasTarget {
					dateKey = fmt.Sprintf("%s-%s-%d", name, eventKey, u.ID)
					dedupUserID = u.ID
				} else {
					// Global fallback: deliver once per event/channel (legacy
					// behavior) no matter how many users share the global target.
					dateKey = fmt.Sprintf("%s-%s", name, eventKey)
					dedupUserID = 0
				}
				exists, err := s.notifLog.ExistsForUser(ctx, name, dateKey, dedupUserID)
				if err != nil {
					slog.Error("scheduler: check notification log", "source", "scheduler", "error", err)
					continue
				}
				if exists {
					slog.Debug("scheduler: notification already sent", "source", "scheduler", "channel", name, "event", event.ID, "user", u.ID)
					continue
				}
				contactName := ""
				if contact := event.Edges.Contact; contact != nil {
					contactName = contact.Name
				} else if p := event.Edges.Person; p != nil {
					contactName = p.Name
				} else if g := event.Edges.Group; g != nil {
					contactName = g.Name + " (group)"
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
					message = fmt.Sprintf("Missed reminder: %s for %s was %s (%s)", event.Type, contactName, occ.Date.Format("January 2"), when)
				} else {
					when := fmt.Sprintf("%d days away", days)
					if days <= 0 {
						when = "today"
					} else if days == 1 {
						when = "tomorrow"
					}
					message = fmt.Sprintf("Upcoming %s for %s on %s (%s)", event.Type, contactName, occ.Date.Format("January 2"), when)
				}
				if err := s.registry.SendTo(ctx, name, title, message, target); err != nil {
					slog.Error("notification failed", "source", "scheduler", "channel", name, "user", u.ID, "error", err)
				}
				if _, err = s.notifLog.CreateForUser(ctx, event.ID, name, dateKey, dedupUserID, time.Now()); err != nil {
					slog.Error("scheduler: log notification", "source", "scheduler", "error", err)
				} else {
					slog.Info("scheduler: notification logged", "source", "scheduler", "channel", name, "event", event.ID, "user", u.ID)
				}
			}
		}
	}
}

func (s *Scheduler) userInScope(u *ent.User, event *ent.Event) bool {
	if u.NotificationScopeMode == "selected" {
		gidStr := u.NotificationScopeGroupIds
		if gidStr == "" {
			return false
		}
		// group-scoped: event must belong to one of selected groups
		if event.Edges.Group == nil {
			return false
		}
		selected := parseGroupIDs(gidStr)
		for _, id := range selected {
			if event.Edges.Group.ID == id {
				return true
			}
		}
		return false
	}
	return true
}

func parseGroupIDs(s string) []int {
	var ids []int
	for _, part := range splitComma(s) {
		var v int
		_, _ = fmt.Sscanf(part, "%d", &v)
		if v != 0 {
			ids = append(ids, v)
		}
	}
	return ids
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i, c := range s {
		if c == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func (s *Scheduler) resolveTarget(ctx context.Context, userID int, channel string) (string, bool) {
	m, err := s.channels.MapByUser(ctx, userID)
	if err != nil {
		return "", false
	}
	if ch, ok := m[channel]; ok && ch.Enabled && ch.Target != "" {
		return ch.Target, true
	}
	if ch, ok := m[channel]; ok && ch.Enabled {
		// enabled but no target means fallback to global
		return "", false
	}
	if _, ok := m[channel]; ok {
		return "", false
	}
	return "", false
}

func (s *Scheduler) hasGlobalFallback(channel string) bool {
	return s.registry.IsConfigured(channel)
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
