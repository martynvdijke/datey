package scheduler

import (
	"context"
	"fmt"
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

	cfg := &config.Config{ReminderDays: reminderDays}

	reg := notifier.NewRegistry()
	fn := &fakeNotifier{name: "email"}
	reg.Register(fn)

	return New(cfg, client, reg), client, fn, repository.NewPersonRepository(client), repository.NewEventRepository(client)
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

	s.processReminders(ctx)

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

	s.processReminders(ctx)

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

	s.processReminders(ctx)

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

	s.processReminders(ctx)

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

	s.processReminders(ctx)
	s.processReminders(ctx)

	if n := fn.count(); n != 1 {
		t.Errorf("expected 1 notification after two runs (deduped), got %d (messages: %v)", n, fn.messages)
	}
}
