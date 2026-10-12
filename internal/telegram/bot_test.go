package telegram

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
)

// fakes

type fakeSender struct {
	msgs []string
	ids  []int64
}

func (f *fakeSender) SendMessage(_ context.Context, chatID int64, text string) error {
	f.msgs = append(f.msgs, text)
	f.ids = append(f.ids, chatID)
	return nil
}
func (f *fakeSender) last() string {
	if len(f.msgs) == 0 {
		return ""
	}
	return f.msgs[len(f.msgs)-1]
}

type fakeEvents struct {
	occs []repository.EventOccurrence
	err  error
}

func (f *fakeEvents) ListUpcomingOccurrences(_ context.Context, _, _ time.Time) ([]repository.EventOccurrence, error) {
	return f.occs, f.err
}

type fakePeople struct {
	byName map[string]*ent.Person
	search map[string][]*ent.Person
	err    error
}

func (f *fakePeople) FindByName(_ context.Context, name string) (*ent.Person, error) {
	if p, ok := f.byName[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakePeople) Search(_ context.Context, q string) ([]*ent.Person, error) {
	if f.err != nil {
		return nil, f.err
	}
	if v, ok := f.search[q]; ok {
		return v, nil
	}
	return nil, nil
}

func newTestBot(occs []repository.EventOccurrence, byName map[string]*ent.Person, search map[string][]*ent.Person) (*Bot, *fakeSender) {
	cfg := &config.Config{ReminderDays: 7, TelegramBotToken: "tok"}
	fe := &fakeEvents{occs: occs}
	fp := &fakePeople{byName: byName, search: search}
	fs := &fakeSender{}
	b := NewWithSender(cfg, fe, fp, fs)
	return b, fs
}

func mkEvent(owner, typ string, date time.Time) *ent.Event {
	e := &ent.Event{Type: typ, Date: date}
	switch owner {
	case "":
	default:
		// Use person edge by default for ownerName
		e.Edges.Person = &ent.Person{Name: owner}
	}
	return e
}

func todayUTC() time.Time { return todayStartUTC() }

func TestHandleUpdate_Help(t *testing.T) {
	b, fs := newTestBot(nil, nil, nil)
	cases := []string{"/help", "/start", "/help@mybot", "/start@mybot"}
	for _, cmd := range cases {
		fs.msgs = nil
		b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 123}, Text: cmd}})
		if len(fs.msgs) != 1 {
			t.Fatalf("cmd %q: expected 1 msg got %d", cmd, len(fs.msgs))
		}
		if !strings.Contains(fs.last(), "/upcoming") || !strings.Contains(fs.last(), "/today") {
			t.Fatalf("cmd %q: help text missing commands: %q", cmd, fs.last())
		}
	}
}

func TestHandleUpdate_UnknownCommand(t *testing.T) {
	b, fs := newTestBot(nil, nil, nil)
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/foobar"}})
	if !strings.Contains(fs.last(), "Unknown command") {
		t.Fatalf("expected unknown command msg, got %q", fs.last())
	}
}

func TestHandleUpdate_NonCommand(t *testing.T) {
	b, fs := newTestBot(nil, nil, nil)
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "hello"}})
	if !strings.Contains(fs.last(), "/help") {
		t.Fatalf("expected /help hint, got %q", fs.last())
	}
}

func TestHandleUpdate_IgnoreNil(t *testing.T) {
	b, fs := newTestBot(nil, nil, nil)
	b.HandleUpdate(context.Background(), nil)
	b.HandleUpdate(context.Background(), &Update{})
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: nil, Text: "/help"}})
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "   "}})
	if len(fs.msgs) != 0 {
		t.Fatalf("expected no msgs, got %v", fs.msgs)
	}
}

func TestUpcoming_DefaultAndExplicit(t *testing.T) {
	today := todayUTC()
	occ1 := repository.EventOccurrence{Event: mkEvent("Ann", "birthday", time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)), Date: today}
	occ2 := repository.EventOccurrence{Event: mkEvent("Bob", "anniversary", time.Date(2010, 6, 15, 0, 0, 0, 0, time.UTC)), Date: today.AddDate(0, 0, 1)}

	// default
	b, fs := newTestBot([]repository.EventOccurrence{occ1, occ2}, nil, nil)
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 10}, Text: "/upcoming"}})
	msg := fs.last()
	if !strings.Contains(msg, "Ann") || !strings.Contains(msg, "Bob") {
		t.Fatalf("upcoming default missing owners: %q", msg)
	}
	if !strings.Contains(msg, "Today") || !strings.Contains(msg, "Tomorrow") {
		t.Fatalf("relative labels missing: %q", msg)
	}
	if !strings.Contains(msg, "turns 36") && !strings.Contains(msg, "turns") { // 2026-1990=36 if today is 2026, but compute dynamically
		// just check turns present for birthday
		if !strings.Contains(msg, "turns") {
			t.Fatalf("expected turns age for birthday: %q", msg)
		}
	}

	// explicit days
	b2, fs2 := newTestBot([]repository.EventOccurrence{occ1}, nil, nil)
	b2.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 10}, Text: "/upcoming 2"}})
	if !strings.Contains(fs2.last(), "Ann") {
		t.Fatalf("explicit days failed: %q", fs2.last())
	}

	// clamp and @bot suffix
	b3, fs3 := newTestBot([]repository.EventOccurrence{occ1}, nil, nil)
	b3.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 10}, Text: "/upcoming@mybot 5"}})
	if !strings.Contains(fs3.last(), "Ann") {
		t.Fatalf("@bot suffix failed: %q", fs3.last())
	}
}

func TestUpcoming_Empty(t *testing.T) {
	b, fs := newTestBot(nil, nil, nil)
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/upcoming"}})
	if !strings.Contains(fs.last(), "No upcoming events") {
		t.Fatalf("expected empty msg, got %q", fs.last())
	}
}

func TestToday(t *testing.T) {
	today := todayUTC()
	occToday := repository.EventOccurrence{Event: mkEvent("Ann", "birthday", time.Date(1990, 3, 4, 0, 0, 0, 0, time.UTC)), Date: today}
	occTomorrow := repository.EventOccurrence{Event: mkEvent("Bob", "birthday", time.Date(1992, 3, 4, 0, 0, 0, 0, time.UTC)), Date: today.AddDate(0, 0, 1)}

	b, fs := newTestBot([]repository.EventOccurrence{occToday, occTomorrow}, nil, nil)
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/today"}})
	msg := fs.last()
	if !strings.Contains(msg, "Ann") {
		t.Fatalf("today should contain Ann: %q", msg)
	}
	if strings.Contains(msg, "Bob") {
		t.Fatalf("today should not contain Bob: %q", msg)
	}

	// empty today
	b2, fs2 := newTestBot([]repository.EventOccurrence{occTomorrow}, nil, nil)
	b2.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/today"}})
	if !strings.Contains(fs2.last(), "Nothing today") {
		t.Fatalf("expected nothing today, got %q", fs2.last())
	}
}

func TestPerson_Found(t *testing.T) {
	today := todayUTC()
	ann := &ent.Person{Name: "Ann"}
	occAnn := repository.EventOccurrence{Event: mkEvent("Ann", "birthday", time.Date(1990, 5, 1, 0, 0, 0, 0, time.UTC)), Date: today}
	occBob := repository.EventOccurrence{Event: mkEvent("Bob", "birthday", time.Date(1985, 5, 1, 0, 0, 0, 0, time.UTC)), Date: today}

	b, fs := newTestBot([]repository.EventOccurrence{occAnn, occBob}, map[string]*ent.Person{"Ann": ann}, nil)
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/person Ann"}})
	msg := fs.last()
	if !strings.Contains(msg, "Ann") {
		t.Fatalf("person found should contain Ann: %q", msg)
	}
	if strings.Contains(msg, "Bob") {
		t.Fatalf("person filter should not contain Bob: %q", msg)
	}
}

func TestPerson_NotFound(t *testing.T) {
	b, fs := newTestBot(nil, nil, map[string][]*ent.Person{"Nobody": nil})
	// search map returns nil slice -> not found
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/person Nobody"}})
	if !strings.Contains(fs.last(), "No person named") {
		t.Fatalf("expected not found, got %q", fs.last())
	}
}

func TestPerson_Ambiguous(t *testing.T) {
	b, fs := newTestBot(nil, nil, map[string][]*ent.Person{
		"Ann": {{Name: "Ann"}, {Name: "Anna"}},
	})
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/person Ann"}})
	if !strings.Contains(fs.last(), "Did you mean") {
		t.Fatalf("expected ambiguous, got %q", fs.last())
	}
}

func TestPerson_SearchFallbackSingle(t *testing.T) {
	today := todayUTC()
	// FindByName fails, Search returns single
	occ := repository.EventOccurrence{Event: mkEvent("Anna", "birthday", time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)), Date: today}
	b, fs := newTestBot([]repository.EventOccurrence{occ}, nil, map[string][]*ent.Person{
		"Anna": {{Name: "Anna"}},
	})
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/person Anna"}})
	if !strings.Contains(fs.last(), "Anna") {
		t.Fatalf("fallback single should resolve: %q", fs.last())
	}
}

func TestPerson_MissingArg(t *testing.T) {
	b, fs := newTestBot(nil, nil, nil)
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/person"}})
	if !strings.Contains(fs.last(), "Usage: /person") {
		t.Fatalf("expected usage, got %q", fs.last())
	}
}

func TestSearch(t *testing.T) {
	today := todayUTC()
	occ := repository.EventOccurrence{Event: mkEvent("Ann", "birthday", time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)), Date: today}
	b, fs := newTestBot([]repository.EventOccurrence{occ}, nil, map[string][]*ent.Person{
		"An": {{Name: "Ann"}, {Name: "Anna"}},
	})
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/search An"}})
	msg := fs.last()
	if !strings.Contains(msg, "Ann") || !strings.Contains(msg, "Anna") {
		t.Fatalf("search should list both: %q", msg)
	}

	// empty
	b2, fs2 := newTestBot(nil, nil, map[string][]*ent.Person{"zzz": nil})
	b2.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/search zzz"}})
	if !strings.Contains(fs2.last(), "No people match") {
		t.Fatalf("expected no match, got %q", fs2.last())
	}

	// missing arg
	b3, fs3 := newTestBot(nil, nil, nil)
	b3.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/search"}})
	if !strings.Contains(fs3.last(), "Usage: /search") {
		t.Fatalf("expected usage, got %q", fs3.last())
	}
}

func TestFormatLine_Milestone(t *testing.T) {
	today := todayUTC()
	// 30th birthday milestone
	e := mkEvent("Ann", "birthday", time.Date(1996, 1, 1, 0, 0, 0, 0, time.UTC))
	occDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	// set occ to 30 years after birth: 2026 if born 1996
	occDate = time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	todayStart := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	line := formatLine(e, occDate, todayStart)
	if !strings.Contains(line, "30th birthday") {
		t.Fatalf("expected milestone, got %q", line)
	}
}
