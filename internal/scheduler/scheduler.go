package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/carddav"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/giftgap"
	"github.com/datey/datey/internal/googlecontacts"
	"github.com/datey/datey/internal/icsub"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/reminderaction"
	"github.com/datey/datey/internal/reminderplan"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/settings"
	"github.com/datey/datey/internal/tzutil"
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
	snoozes  *repository.SnoozeRepository
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
		snoozes:  repository.NewSnoozeRepository(client),
	}
}

// actionSecret returns the signing key for reminder action links: the first
// configured feed key, matching internal/web's share-token key order. A nil
// result means action links are unavailable and reminders go out without them.
func (s *Scheduler) actionSecret() []byte {
	for _, k := range []string{s.cfg.ICalFeedKey, s.cfg.UpcomingAPIKey, s.cfg.RSSFeedKey} {
		if k != "" {
			return []byte(k)
		}
	}
	return nil
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

	// Live ICS subscription polling runs on its own cadence, independent of the
	// daily reminder pass. Zero disables automatic polling.
	if interval := time.Duration(s.cfg.SubscriptionPollHours) * time.Hour; interval > 0 {
		go func() {
			s.runSubscriptionSync(ctx)
			t := time.NewTicker(interval)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					s.runSubscriptionSync(ctx)
				}
			}
		}()
	}

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
			s.runGoogleSync(ctx)
			s.pruneAuditLog(ctx)
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

func (s *Scheduler) runGoogleSync(ctx context.Context) {
	if !s.cfg.GoogleContactsEnabled || s.cfg.GoogleClientID == "" || s.cfg.GoogleRefreshToken == "" {
		return
	}
	lastSync, err := s.settings.GoogleLastSync(ctx)
	if err != nil {
		slog.Error("scheduler: read google last sync", "source", "scheduler", "error", err)
		return
	}
	if lastSync != nil && time.Since(*lastSync) < 24*time.Hour {
		slog.Debug("scheduler: google sync not due", "source", "scheduler", "last_sync", lastSync.Format(time.RFC3339))
		return
	}
	slog.Info("scheduler: running google sync", "source", "scheduler")
	oauthCfg := googlecontacts.OAuthConfig(s.cfg.GoogleClientID, s.cfg.GoogleClientSecret, "")
	ts := googlecontacts.TokenSource(ctx, oauthCfg, s.cfg.GoogleRefreshToken)
	transport := &googlecontacts.OAuth2Transport{TokenSource: ts}
	client := googlecontacts.New(transport)
	syncer := googlecontacts.NewSyncer(s.cfg, s.client, s.settings, client)
	if _, err := syncer.Sync(ctx); err != nil {
		slog.Error("scheduler: google sync failed", "source", "scheduler", "error", err)
	}
}

// runSubscriptionSync polls every enabled ICS subscription and applies the
// UID-keyed diff. A failed fetch is logged and leaves the stored events
// untouched, so an unreachable feed never destroys data.
func (s *Scheduler) runSubscriptionSync(ctx context.Context) {
	if s.cfg.SubscriptionPollHours <= 0 {
		return
	}
	summary, err := icsub.NewSyncer(s.client, nil).SyncAllEnabled(ctx)
	if err != nil {
		slog.Error("scheduler: subscription sync", "source", "scheduler", "error", err)
		return
	}
	slog.Info("scheduler: subscriptions synced", "source", "scheduler",
		"created", summary.Created, "updated", summary.Updated, "deleted", summary.Deleted)
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

	from := now.AddDate(0, 0, -s.reminderHorizon())
	if lastRun != nil {
		from = lastRun.AddDate(0, 0, -s.reminderHorizon())
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
	base := strings.TrimRight(s.cfg.AppURL, "/")
	sender := notifier.NewSender(notifier.NewHealthRecorder(repository.NewChannelHealthRepository(s.client)))
	now := time.Now()
	from := tzutil.Today(now, tzutil.Default()).AddDate(0, 0, -1)
	end := from.AddDate(0, 0, s.reminderHorizon()+2)
	if catchUp {
		end = now
		from = now.AddDate(0, 0, -s.reminderHorizon()-1)
		if lastRun, err := s.settings.LastSchedulerRun(ctx); err == nil && lastRun != nil {
			from = lastRun.AddDate(0, 0, -s.reminderHorizon())
		}
	}
	occurrences, err := s.events.ListUpcomingOccurrences(ctx, from, end)
	if err != nil {
		slog.Error("scheduler: list upcoming events", "source", "scheduler", "error", err)
		return
	}
	slog.Info("processing reminders", "source", "scheduler", "event_count", len(occurrences), "catch_up", catchUp)
	if s.cfg.ReminderDigest {
		s.sendDigest(ctx, occurrences, now, sender)
		return
	}
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
			if snoozed, serr := s.snoozes.Active(ctx, event.ID, occ.Date, 0, now); serr != nil {
				slog.Error("scheduler: check snooze", "source", "scheduler", "error", serr)
			} else if snoozed {
				slog.Debug("scheduler: occurrence snoozed", "source", "scheduler", "event", event.ID, "user", 0)
				continue
			}
			eventKey := fmt.Sprintf("%d-%s", event.ID, occ.Date.Format("2006-01-02"))
			days := daysBetween(now, occ.Date, eventLocation(event))
			dayOf := days == 0
			stages := s.occurrenceStages(event)
			stage, staged := stageForDays(days, stages)
			if !staged && !dayOf && (!catchUp || days >= 0) {
				continue
			}
			if len(stages) > 1 {
				eventKey = fmt.Sprintf("%s-%d", eventKey, stage)
			}
			var actions notifier.ActionLinks
			if secret := s.actionSecret(); secret != nil {
				if su, au, lerr := reminderaction.Links(secret, base, event.ID, occ.Date, 0, reminderaction.DefaultTTL); lerr != nil {
					slog.Error("scheduler: mint action links", "source", "scheduler", "error", lerr, "event", event.ID)
				} else {
					actions = notifier.ActionLinks{SnoozeURL: su, AckURL: au}
				}
			}
			for _, name := range []string{"email", "gotify", "telegram", "ntfy", "pushover", "webhook", "webpush", "discord", "slack", "matrix"} {
				if !s.registry.IsConfigured(name) {
					continue
				}
				dateKey := reminderLogKey(name, eventKey, dayOf && !catchUp, 0)
				var exists bool
				var err error
				if catchUp {
					exists, err = s.occurrenceAnnounced(ctx, name, eventKey, 0)
				} else {
					exists, err = s.notifLog.ExistsForDate(ctx, name, dateKey)
				}
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
				} else if event.RuleKind != "" && event.Description != "" {
					// Calendar-rule events carry their name in the description.
					contactName = event.Description
				}
				title := fmt.Sprintf("Reminder: %s - %s", contactName, event.Type)
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
				if extra := s.giftGapLine(ctx, event); extra != "" {
					message += "\n" + extra
				}
				if p := event.Edges.Person; p != nil {
					if link := notifier.PersonLinkLine(s.cfg.AppURL, p.ID); link != "" {
						message += "\n" + link
					}
				}
				if n, ok := s.registry.Notifier(name); ok {
					_ = sender.SendToActions(ctx, n, title, message, "", actions)
				}
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
		days := daysBetween(now, occ.Date, eventLocation(event))
		dayOf := days == 0
		stages := s.occurrenceStages(event)
		stage, staged := stageForDays(days, stages)
		if !staged && !dayOf && (!catchUp || days >= 0) {
			continue
		}
		if len(stages) > 1 {
			eventKey = fmt.Sprintf("%s-%d", eventKey, stage)
		}
		for _, u := range users {
			if !s.userInScope(u, event) {
				continue
			}
			if snoozed, serr := s.snoozes.Active(ctx, event.ID, occ.Date, u.ID, now); serr != nil {
				slog.Error("scheduler: check snooze", "source", "scheduler", "error", serr)
			} else if snoozed {
				slog.Debug("scheduler: occurrence snoozed", "source", "scheduler", "event", event.ID, "user", u.ID)
				continue
			}
			var actions notifier.ActionLinks
			if secret := s.actionSecret(); secret != nil {
				if su, au, lerr := reminderaction.Links(secret, base, event.ID, occ.Date, u.ID, reminderaction.DefaultTTL); lerr != nil {
					slog.Error("scheduler: mint action links", "source", "scheduler", "error", lerr, "event", event.ID)
				} else {
					actions = notifier.ActionLinks{SnoozeURL: su, AckURL: au}
				}
			}
			for _, name := range []string{"email", "gotify", "telegram", "ntfy", "pushover", "webhook", "webpush", "discord", "slack", "matrix"} {
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
					dedupUserID = u.ID
				}
				dateKey = reminderLogKey(name, eventKey, dayOf && !catchUp, dedupUserID)
				var exists bool
				var err error
				if catchUp {
					exists, err = s.occurrenceAnnounced(ctx, name, eventKey, dedupUserID)
				} else {
					exists, err = s.notifLog.ExistsForUser(ctx, name, dateKey, dedupUserID)
				}
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
				} else if event.RuleKind != "" && event.Description != "" {
					// Calendar-rule events carry their name in the description.
					contactName = event.Description
				}
				title := fmt.Sprintf("Reminder: %s - %s", contactName, event.Type)
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
				if extra := s.giftGapLine(ctx, event); extra != "" {
					message += "\n" + extra
				}
				if p := event.Edges.Person; p != nil {
					if link := notifier.PersonLinkLine(s.cfg.AppURL, p.ID); link != "" {
						message += "\n" + link
					}
				}
				if n, ok := s.registry.Notifier(name); ok {
					if err := sender.SendToActions(ctx, n, title, message, target, actions); err != nil {
						slog.Error("notification failed", "source", "scheduler", "channel", name, "user", u.ID, "error", err)
					}
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

// reminderLogKey builds a notification_log date key. base is
// "<eventID>-<occurrenceDate>". A reminder that falls on the occurrence day
// itself carries a "-dayof" suffix so it still fires when the advance (entry)
// reminder already went out days earlier; both keys share the occurrence base
// so a catch-up pass can tell the occurrence was already announced. userID 0
// is the legacy global scope and adds no suffix.
func reminderLogKey(channel, base string, dayOf bool, userID int) string {
	key := fmt.Sprintf("%s-%s", channel, base)
	if dayOf {
		key += "-dayof"
	}
	if userID != 0 {
		key = fmt.Sprintf("%s-%d", key, userID)
	}
	return key
}

// occurrenceAnnounced reports whether any reminder (entry or day-of) was ever
// logged for an occurrence, so a catch-up pass does not repeat it.
func (s *Scheduler) occurrenceAnnounced(ctx context.Context, channel, base string, userID int) (bool, error) {
	if ok, err := s.notifLog.ExistsForUser(ctx, channel, reminderLogKey(channel, base, false, userID), userID); err != nil || ok {
		return ok, err
	}
	return s.notifLog.ExistsForUser(ctx, channel, reminderLogKey(channel, base, true, userID), userID)
}

// daysBetween returns the calendar-day difference between now and t: how many
// days t is after now (negative when before). Both are truncated to their UTC
// date, so an occurrence later today reads 0 ("today") and one tomorrow reads
// 1, regardless of the pass time of day.
// giftGapLine returns the optional "No gift idea yet" suffix for a birthday or
// anniversary whose person has no open gift idea. It returns "" for events
// that are not celebrations, events without a person, and persons that already
// have an open ("idea") gift idea.
func (s *Scheduler) giftGapLine(ctx context.Context, event *ent.Event) string {
	if event.Type != "birthday" && event.Type != "anniversary" {
		return ""
	}
	p := event.Edges.Person
	if p == nil {
		return ""
	}
	n, err := repository.NewGiftIdeaRepository(s.client).CountOpenByPerson(ctx, p.ID)
	if err != nil {
		return ""
	}
	return giftgap.Line(p.ID, n, s.cfg.AppURL)
}

// occurrenceStages resolves the effective reminder stages for an event:
// event override, then person override, then the global list.
func (s *Scheduler) occurrenceStages(e *ent.Event) []int {
	var p []int
	if e.Edges.Person != nil {
		p = e.Edges.Person.ReminderDays
	}
	stages := reminderplan.EffectiveStages(s.cfg.ReminderStages, p, e.ReminderDays)
	if len(stages) == 0 {
		// Configs built without Load() (tests, embedders) carry no stage list.
		// Fall back to the single legacy lead time so reminders still fire.
		if s.cfg.ReminderDays > 0 {
			return []int{s.cfg.ReminderDays}
		}
	}
	return stages
}

// stageForDays reports which reminder stage (in days before the occasion) the
// current pass belongs to. A pass also matches when the occasion is already
// closer than the next stage, so a stage is not silently skipped. Day counts
// are supplied by the caller, already computed in the event owner's zone.
func stageForDays(days int, stages []int) (int, bool) {
	if days < 1 || len(stages) == 0 {
		return 0, false
	}
	best := 0
	for _, st := range stages {
		if st == days {
			return st, true
		}
		if st >= days && (best == 0 || st < best) {
			best = st
		}
	}
	return best, best != 0
}

// eventLocation picks the zone a person's dates are counted in: their own
// timezone when set, otherwise the instance-wide default.
func eventLocation(e *ent.Event) *time.Location {
	if e != nil && e.Edges.Person != nil {
		return tzutil.Resolve(e.Edges.Person.Timezone)
	}
	return tzutil.Default()
}

// reminderHorizon is the widest look-ahead the occurrence query must cover so
// every configured stage can fire: the larger of REMINDER_DAYS and the
// largest configured stage.
func (s *Scheduler) reminderHorizon() int {
	h := s.cfg.ReminderDays
	for _, st := range s.cfg.ReminderStages {
		if st > h {
			h = st
		}
	}
	if h < 1 {
		h = 1
	}
	return h
}

// sendDigest delivers one combined message per configured channel instead of
// one message per event, deduplicated by channel and day.
func (s *Scheduler) sendDigest(ctx context.Context, occs []repository.EventOccurrence, now time.Time, sender *notifier.Sender) {
	dateKey := now.Format("2006-01-02")
	entries := make([]reminderplan.DigestEntry, 0, len(occs))
	anchor := 0
	for _, occ := range occs {
		e := occ.Event
		if e.Type == "birthday" && e.Edges.Person != nil && !e.Edges.Person.NotifyBirthdays {
			continue
		}
		days := daysBetween(now, occ.Date, eventLocation(e))
		if days < 0 {
			continue
		}
		if anchor == 0 {
			anchor = e.ID
		}
		name := e.Type
		if c := e.Edges.Contact; c != nil {
			name = c.Name + " - " + e.Type
		} else if p := e.Edges.Person; p != nil {
			name = p.Name + " - " + e.Type
		} else if g := e.Edges.Group; g != nil {
			name = g.Name + " (group) - " + e.Type
		} else if e.RuleKind != "" && e.Description != "" {
			name = e.Description + " - " + e.Type
		}
		when := "today"
		if days == 1 {
			when = "tomorrow"
		} else if days > 1 {
			when = fmt.Sprintf("%d days away", days)
		}
		entries = append(entries, reminderplan.DigestEntry{
			Title:   name,
			Message: fmt.Sprintf("%s (%s)", occ.Date.Format("January 2"), when),
		})
	}
	body := reminderplan.ComposeDigest(entries)
	if body == "" {
		body = "No upcoming events in the reminder window."
	}
	for _, name := range []string{"email", "gotify", "telegram", "ntfy", "pushover", "webhook", "webpush", "discord", "slack", "matrix"} {
		if !s.registry.IsConfigured(name) {
			continue
		}
		key := reminderplan.DigestKey(name, dateKey)
		if ex, err := s.notifLog.ExistsForDate(ctx, name, key); err != nil || ex {
			continue
		}
		if n, ok := s.registry.Notifier(name); ok {
			_ = sender.Send(ctx, n, reminderplan.DigestHeader, body)
		}
		if anchor != 0 {
			_, _ = s.notifLog.Create(ctx, anchor, name, key, time.Now())
		}
	}
}

func daysBetween(now, t time.Time, loc *time.Location) int {
	return tzutil.DaysBetween(tzutil.Today(now, loc), t)
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
func (s *Scheduler) pruneAuditLog(ctx context.Context) {
	cap := s.cfg.AuditRetentionMax
	if cap < 100 {
		cap = 10000
	}
	if cap == 0 {
		cap = 10000
	}
	repo := repository.NewAuditEntryRepository(s.client)
	if n, err := repo.PruneToCap(ctx, cap); err != nil {
		slog.Warn("scheduler: audit prune failed", "error", err)
	} else if n > 0 {
		slog.Info("scheduler: audit log pruned", "removed", n, "cap", cap)
	}
}

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
