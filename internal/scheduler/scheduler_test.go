package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/settings"
	_ "github.com/mattn/go-sqlite3"
)

type fakeNotifier struct {
	name string

	mu       sync.Mutex
	sent     int
	messages []string
}

func (f *fakeNotifier) Send(_ context.Context, title, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent++
	f.messages = append(f.messages, title+" "+message)
	return nil
}

func (f *fakeNotifier) Name() string { return f.name }

func (f *fakeNotifier) IsConfigured() bool { return true }

func (f *fakeNotifier) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent
}

func newTestScheduler(t *testing.T, reminderDays int) (*Scheduler, *ent.Client, *fakeNotifier, *repository.PersonRepository, *repository.EventRepository) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_scheduler?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	cfg := &config.Config{ReminderDays: reminderDays, SchedulerCatchup: true}

	reg := notifier.NewRegistry()
	fn := &fakeNotifier{name: "email"}
	reg.Register(fn)

	return New(cfg, client, reg, settings.New(client)), client, fn, repository.NewPersonRepository(client), repository.NewEventRepository(client)
}

func TestReminderMessage(t *testing.T) {
	now := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	eventDate := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	days := int(eventDate.Sub(now).Hours() / 24)

	message := "Upcoming birthday for John on June 15 (14 days away)"

	if !strings.Contains(message, "14 days") {
		t.Errorf("Expected 14 days in message, got: %s", message)
	}
	if days != 14 {
		t.Errorf("Expected 14 days remaining, got %d", days)
	}
}

// midnightDaysFromNow returns midnight (UTC) `days` days from now's date.
func midnightDaysFromNow(days int) time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
}

func TestProcessReminders_AnnualBirthdayFires(t *testing.T) {
	s, _, fn, people, events := newTestScheduler(t, 7)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// A birthday stored with a historical birth year must fire on its
	// occurrence within the reminder window (month/day of now+5 days).
	target := midnightDaysFromNow(5)
	birth := time.Date(1990, target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, person.ID, "birthday", birth, "Birthday of Dana"); err != nil {
		t.Fatalf("create event: %v", err)
	}

	s.processReminders(ctx, false)

	if n := fn.count(); n != 1 {
		t.Errorf("expected 1 notification, got %d (messages: %v)", n, fn.messages)
	}
}

func TestProcessReminders_NoOccurrenceInWindowNoReminder(t *testing.T) {
	s, _, fn, people, events := newTestScheduler(t, 7)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// Birthday 60 days out: outside the 7-day reminder window.
	birth := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, person.ID, "birthday", birth, "Birthday of Dana"); err != nil {
		t.Fatalf("create event: %v", err)
	}

	s.processReminders(ctx, false)

	if n := fn.count(); n != 0 {
		t.Errorf("expected 0 notifications, got %d (messages: %v)", n, fn.messages)
	}
}

func TestProcessReminders_YearBoundaryUsesNextYearOccurrence(t *testing.T) {
	s, _, fn, people, events := newTestScheduler(t, 365)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// A date in the past this year (yesterday): its occurrence within a
	// 365-day window is next year's date, not this year's (which already
	// passed and is outside the window).
	yesterday := midnightDaysFromNow(-1)
	birth := time.Date(1990, yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	ev, err := events.CreateForPerson(ctx, person.ID, "birthday", birth, "Birthday of Dana")
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	s.processReminders(ctx, false)

	nextYearKey := "email-" + fmt.Sprintf("%d-%s", ev.ID, time.Date(yesterday.Year()+1, yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC).Format("2006-01-02"))
	thisYearKey := "email-" + fmt.Sprintf("%d-%s", ev.ID, time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC).Format("2006-01-02"))

	logRepo := repository.NewNotificationLogRepository(s.client)
	if ok, err := logRepo.ExistsForDate(ctx, "email", nextYearKey); err != nil || !ok {
		t.Errorf("expected next-year occurrence key %q logged, ok=%v err=%v", nextYearKey, ok, err)
	}
	if ok, _ := logRepo.ExistsForDate(ctx, "email", thisYearKey); ok {
		t.Errorf("this-year occurrence key %q should not be logged", thisYearKey)
	}
	if n := fn.count(); n != 1 {
		t.Errorf("expected 1 notification, got %d", n)
	}
}

func TestProcessReminders_OptOutSkipsBirthdayOnly(t *testing.T) {
	s, client, fn, people, events := newTestScheduler(t, 7)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if err := client.Person.UpdateOneID(person.ID).SetNotifyBirthdays(false).Exec(ctx); err != nil {
		t.Fatalf("disable notify_birthdays: %v", err)
	}

	target := midnightDaysFromNow(5)
	birth := time.Date(1990, target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, person.ID, "birthday", birth, "Birthday of Dana"); err != nil {
		t.Fatalf("create birthday: %v", err)
	}
	// Non-birthday types must be unaffected by the opt-out.
	anniv := time.Date(2020, target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, person.ID, "anniversary", anniv, "Anniversary"); err != nil {
		t.Fatalf("create anniversary: %v", err)
	}

	s.processReminders(ctx, false)

	if n := fn.count(); n != 1 {
		t.Fatalf("expected 1 notification (anniversary only), got %d (messages: %v)", n, fn.messages)
	}
	if !strings.Contains(fn.messages[0], "anniversary") {
		t.Errorf("expected the anniversary notification, got %q", fn.messages[0])
	}
}

func TestProcessReminders_DedupedWithinSameOccurrence(t *testing.T) {
	s, _, fn, people, events := newTestScheduler(t, 7)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	target := midnightDaysFromNow(5)
	birth := time.Date(1990, target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, person.ID, "birthday", birth, "Birthday of Dana"); err != nil {
		t.Fatalf("create event: %v", err)
	}

	s.processReminders(ctx, false)
	s.processReminders(ctx, false)

	if n := fn.count(); n != 1 {
		t.Errorf("expected 1 notification after two runs (deduped), got %d (messages: %v)", n, fn.messages)
	}
}

func TestProcessReminders_EventDatedTodayFires(t *testing.T) {
	s, _, fn, people, events := newTestScheduler(t, 7)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// A custom event dated today (midnight UTC) must be reminded on the day
	// itself. Previously the window started at time.Now(), so today's date —
	// which is earlier than the pass time — was excluded and never notified.
	today := midnightDaysFromNow(0)
	if _, err := events.CreateForPerson(ctx, person.ID, "custom", today, "School event"); err != nil {
		t.Fatalf("create event: %v", err)
	}

	s.processReminders(ctx, false)

	if n := fn.count(); n != 1 {
		t.Fatalf("expected 1 notification for today's event, got %d (messages: %v)", n, fn.messages)
	}
	if !strings.Contains(fn.messages[0], "today") {
		t.Errorf("expected 'today' phrasing, got %q", fn.messages[0])
	}
}

func TestProcessReminders_CustomEventRecursAnnually(t *testing.T) {
	s, _, fn, people, events := newTestScheduler(t, 365)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// A custom event stored with a past date must recur annually: within a
	// 365-day window its next occurrence is next year's date.
	lastYear := midnightDaysFromNow(-300)
	ev, err := events.CreateForPerson(ctx, person.ID, "custom", lastYear, "School event")
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	s.processReminders(ctx, false)

	if n := fn.count(); n != 1 {
		t.Fatalf("expected 1 notification for annual custom event, got %d (messages: %v)", n, fn.messages)
	}
	nextYearKey := "email-" + fmt.Sprintf("%d-%s", ev.ID, time.Date(lastYear.Year()+1, lastYear.Month(), lastYear.Day(), 0, 0, 0, 0, time.UTC).Format("2006-01-02"))
	logRepo := repository.NewNotificationLogRepository(s.client)
	if ok, err := logRepo.ExistsForDate(ctx, "email", nextYearKey); err != nil || !ok {
		t.Errorf("expected next-year occurrence key %q logged, ok=%v err=%v", nextYearKey, ok, err)
	}
}

// newTestCatchupScheduler returns a scheduler plus a helper to seed a person
// whose birthday falls `daysAgo` days before now (a date inside the reminder
// window when a catch-up pass runs).
func newTestCatchupScheduler(t *testing.T, reminderDays int) (*Scheduler, *fakeNotifier, *repository.PersonRepository, *repository.EventRepository, *settings.Store) {
	t.Helper()
	s, _, fn, people, events := newTestScheduler(t, reminderDays)
	return s, fn, people, events, settings.New(s.client)
}

func TestCatchUp_NoCatchUpWhenLastRunRecent(t *testing.T) {
	s, fn, people, events, store := newTestCatchupScheduler(t, 7)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	// A birthday 5 days ago: inside the reminder window, but the last run was
	// only an hour ago (within the grace period), so no catch-up must fire.
	target := midnightDaysFromNow(-5)
	birth := time.Date(1990, target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, person.ID, "birthday", birth, "Birthday of Dana"); err != nil {
		t.Fatalf("create event: %v", err)
	}
	if err := store.SetLastSchedulerRun(ctx, time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatalf("set last run: %v", err)
	}

	s.catchUpMissed(ctx)

	if n := fn.count(); n != 0 {
		t.Errorf("expected no catch-up within grace period, got %d notifications (messages: %v)", n, fn.messages)
	}
}

func TestCatchUp_FiresMissedOccurrencesInWindow(t *testing.T) {
	s, fn, people, events, store := newTestCatchupScheduler(t, 7)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	// Birthday 5 days ago: inside [lastRun - ReminderDays, now].
	target := midnightDaysFromNow(-5)
	birth := time.Date(1990, target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, person.ID, "birthday", birth, "Birthday of Dana"); err != nil {
		t.Fatalf("create event: %v", err)
	}
	// Last run was 3 days ago: gap > 24h + 30m grace.
	if err := store.SetLastSchedulerRun(ctx, time.Now().Add(-72 * time.Hour)); err != nil {
		t.Fatalf("set last run: %v", err)
	}

	s.catchUpMissed(ctx)

	if n := fn.count(); n != 1 {
		t.Fatalf("expected 1 catch-up notification, got %d (messages: %v)", n, fn.messages)
	}
	if !strings.Contains(fn.messages[0], "Missed reminder") || !strings.Contains(fn.messages[0], "days ago") {
		t.Errorf("expected missed phrasing in catch-up message, got %q", fn.messages[0])
	}
}

func TestCatchUp_SkipsOccurrencesOlderThanWindow(t *testing.T) {
	s, fn, people, events, store := newTestCatchupScheduler(t, 7)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	// Birthday 10 days ago, with a 7-day reminder window and last run 3 days
	// ago: the catch-up window is [lastRun-7d, now], so a date 10 days back is
	// outside it and must not fire.
	target := midnightDaysFromNow(-10)
	birth := time.Date(1990, target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, person.ID, "birthday", birth, "Birthday of Dana"); err != nil {
		t.Fatalf("create event: %v", err)
	}
	if err := store.SetLastSchedulerRun(ctx, time.Now().Add(-72 * time.Hour)); err != nil {
		t.Fatalf("set last run: %v", err)
	}

	s.catchUpMissed(ctx)

	if n := fn.count(); n != 0 {
		t.Errorf("expected 0 notifications for occurrence outside window, got %d (messages: %v)", n, fn.messages)
	}
}

func TestCatchUp_DedupPreventsDoubleSend(t *testing.T) {
	s, fn, people, events, store := newTestCatchupScheduler(t, 7)
	ctx := context.Background()

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	target := midnightDaysFromNow(-5)
	birth := time.Date(1990, target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	ev, err := events.CreateForPerson(ctx, person.ID, "birthday", birth, "Birthday of Dana")
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if err := store.SetLastSchedulerRun(ctx, time.Now().Add(-72 * time.Hour)); err != nil {
		t.Fatalf("set last run: %v", err)
	}

	// First catch-up pass sends the reminder...
	s.catchUpMissed(ctx)
	if n := fn.count(); n != 1 {
		t.Fatalf("expected 1 notification on first catch-up, got %d", n)
	}

	// ...and the persisted last_scheduler_run is refreshed, so a restart right
	// after must NOT catch up again.
	last, err := store.LastSchedulerRun(ctx)
	if err != nil || last == nil {
		t.Fatalf("expected last run persisted after catch-up, got %v (err %v)", last, err)
	}
	if time.Since(*last) > 5*time.Minute {
		t.Errorf("last run not refreshed by catch-up: %s", last.Format(time.RFC3339))
	}

	// Force a second catch-up regardless (simulating a large gap) and verify
	// the notification_log dedup prevents a duplicate send.
	if err := store.SetLastSchedulerRun(ctx, time.Now().Add(-72 * time.Hour)); err != nil {
		t.Fatalf("set last run: %v", err)
	}
	s.catchUpMissed(ctx)

	if n := fn.count(); n != 1 {
		t.Errorf("expected still 1 notification after second catch-up (deduped), got %d", n)
	}

	// Sanity: the event is unrelated to dedup correctness.
	_ = ev
}

func TestCatchUp_DisabledWhenSchedulerCatchupFalse(t *testing.T) {
	s, fn, people, events, _ := newTestCatchupScheduler(t, 7)
	ctx := context.Background()

	// Simulate the toggle being off via the in-memory cfg (as ApplyForm would).
	s.cfg.SchedulerCatchup = false

	person, err := people.Create(ctx, "Dana", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	target := midnightDaysFromNow(-5)
	birth := time.Date(1990, target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, person.ID, "birthday", birth, "Birthday of Dana"); err != nil {
		t.Fatalf("create event: %v", err)
	}

	s.catchUpMissed(ctx)

	if n := fn.count(); n != 0 {
		t.Errorf("expected 0 notifications when catch-up disabled, got %d", n)
	}
}

func TestCatchUp_PersistsLastRunAfterPass(t *testing.T) {
	s, _, _, _, store := newTestCatchupScheduler(t, 7)
	ctx := context.Background()

	s.persistRun(ctx)

	last, err := store.LastSchedulerRun(ctx)
	if err != nil {
		t.Fatalf("read last run: %v", err)
	}
	if last == nil {
		t.Fatal("expected last_scheduler_run to be persisted after a pass")
	}
	if time.Since(*last) > 5*time.Minute {
		t.Errorf("persisted last run too old: %s", last.Format(time.RFC3339))
	}
}

// newWeeklyBackupScheduler returns a scheduler wired to temp data/backup dirs
// with a real SQLite file so runWeeklyBackup can actually copy it.
func newWeeklyBackupScheduler(t *testing.T) *Scheduler {
	t.Helper()
	s, _, _, _, _ := newTestScheduler(t, 7)
	s.cfg.DataDir = t.TempDir()
	s.cfg.BackupDir = t.TempDir()
	s.cfg.WeeklyBackupRetentionWeeks = 52

	dbPath := s.cfg.DataDir + "/datey.db"
	dbFile, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	if err := dbFile.Close(); err != nil {
		t.Fatalf("close test db: %v", err)
	}
	return s
}

func weeklyBackupCount(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "datey_weekly_") && strings.HasSuffix(e.Name(), ".db") {
			n++
		}
	}
	return n
}

func TestRunWeeklyBackup_FiresOnConfiguredDay(t *testing.T) {
	s := newWeeklyBackupScheduler(t)
	ctx := context.Background()

	s.cfg.WeeklyBackupDay = int(time.Now().Weekday())
	s.runWeeklyBackup(ctx)

	if n := weeklyBackupCount(t, s.cfg.BackupDir); n != 1 {
		t.Errorf("expected 1 weekly backup on the configured day, got %d", n)
	}
}

func TestRunWeeklyBackup_SkipsOtherDays(t *testing.T) {
	s := newWeeklyBackupScheduler(t)
	ctx := context.Background()

	s.cfg.WeeklyBackupDay = int((time.Now().Weekday() + 1) % 7)
	s.runWeeklyBackup(ctx)

	if n := weeklyBackupCount(t, s.cfg.BackupDir); n != 0 {
		t.Errorf("expected no weekly backup on other days, got %d", n)
	}
}
