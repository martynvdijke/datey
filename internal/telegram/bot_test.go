package telegram

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/search"
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

	lastFrom time.Time
	lastTo   time.Time
}

func (f *fakeEvents) ListUpcomingOccurrences(_ context.Context, from, to time.Time) ([]repository.EventOccurrence, error) {
	f.lastFrom, f.lastTo = from, to
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

type fakeSearcher struct {
	groups   []search.Group
	err      error
	gotQuery string
}

func (f *fakeSearcher) Search(_ context.Context, q string) ([]search.Group, error) {
	f.gotQuery = q
	return f.groups, f.err
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
	if owner != "" {
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

// TestHandleUpdate_Parsing covers the shapes Telegram sends: @BotName suffixes,
// extra whitespace, mixed case and commands without arguments.
func TestHandleUpdate_Parsing(t *testing.T) {
	today := todayUTC()
	occ := repository.EventOccurrence{Event: mkEvent("Ann", "birthday", time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)), Date: today}

	cases := []struct {
		text  string
		check string
	}{
		{"/UPCOMING", "Ann"},
		{"  /upcoming   ", "Ann"},
		{"/upcoming@datey_bot", "Ann"},
		{"/UpComing@DateyBot   5", "Ann"},
		{"/today@datey_bot", "Ann"},
		{"/help@datey_bot", "/upcoming"},
	}
	for _, tc := range cases {
		b, fs := newTestBot([]repository.EventOccurrence{occ}, nil, nil)
		b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 7}, Text: tc.text}})
		if !strings.Contains(fs.last(), tc.check) {
			t.Fatalf("text %q: expected %q in reply, got %q", tc.text, tc.check, fs.last())
		}
		if fs.ids[0] != 7 {
			t.Fatalf("text %q: wrong chat id %d", tc.text, fs.ids[0])
		}
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
	if !strings.Contains(msg, "turns") {
		t.Fatalf("expected turns age for birthday: %q", msg)
	}

	// explicit days
	b2, fs2 := newTestBot([]repository.EventOccurrence{occ1}, nil, nil)
	b2.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 10}, Text: "/upcoming 2"}})
	if !strings.Contains(fs2.last(), "Ann") {
		t.Fatalf("explicit days failed: %q", fs2.last())
	}

	// @bot suffix
	b3, fs3 := newTestBot([]repository.EventOccurrence{occ1}, nil, nil)
	b3.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 10}, Text: "/upcoming@mybot 5"}})
	if !strings.Contains(fs3.last(), "Ann") {
		t.Fatalf("@bot suffix failed: %q", fs3.last())
	}
}

func TestUpcoming_Clamp(t *testing.T) {
	today := todayUTC()
	occ := repository.EventOccurrence{Event: mkEvent("Ann", "birthday", time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)), Date: today}

	cases := []struct {
		arg      string
		wantDays int
	}{
		{"", defaultUpcomingDays},
		{"0", 1},
		{"-5", 1},
		{"9999", 365},
		{"abc", defaultUpcomingDays},
	}
	for _, tc := range cases {
		fe := &fakeEvents{occs: []repository.EventOccurrence{occ}}
		b := NewWithSender(&config.Config{ReminderDays: 7}, fe, &fakePeople{}, &fakeSender{})
		b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/upcoming " + tc.arg}})
		gotDays := int(fe.lastTo.Sub(fe.lastFrom).Hours() / 24)
		if gotDays != tc.wantDays {
			t.Fatalf("arg %q: expected window %d days, got %d", tc.arg, tc.wantDays, gotDays)
		}
	}
}

func TestUpcoming_CapsRenderedLines(t *testing.T) {
	today := todayUTC()
	occs := make([]repository.EventOccurrence, 0, maxUpcomingLines+5)
	for i := 0; i < maxUpcomingLines+5; i++ {
		occs = append(occs, repository.EventOccurrence{
			Event: mkEvent(fmt.Sprintf("P%d", i), "birthday", today.AddDate(-30, 0, 0)),
			Date:  today.AddDate(0, 0, i),
		})
	}
	b, fs := newTestBot(occs, nil, nil)
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/upcoming 60"}})
	if !strings.Contains(fs.last(), "and 5 more") {
		t.Fatalf("expected overflow marker, got %q", fs.last())
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
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/person Nobody"}})
	if !strings.Contains(fs.last(), "No person named") {
		t.Fatalf("expected not found, got %q", fs.last())
	}
}

func TestPerson_Ambiguous(t *testing.T) {
	people := make([]*ent.Person, 0, maxPersonCandidates+3)
	for i := 0; i < maxPersonCandidates+3; i++ {
		people = append(people, &ent.Person{Name: fmt.Sprintf("Ann %d", i)})
	}
	b, fs := newTestBot(nil, nil, map[string][]*ent.Person{"Ann": people})
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/person Ann"}})
	msg := fs.last()
	if !strings.Contains(msg, "Did you mean") {
		t.Fatalf("expected ambiguous, got %q", msg)
	}
	if !strings.Contains(msg, "and 3 more") {
		t.Fatalf("expected candidate cap marker, got %q", msg)
	}
	if !strings.Contains(msg, "more specific") {
		t.Fatalf("expected more-specific hint, got %q", msg)
	}
}

func TestPerson_SearchFallbackSingle(t *testing.T) {
	today := todayUTC()
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

func searchGroups() []search.Group {
	return []search.Group{
		{
			Entity: search.EntityPerson,
			Results: []search.Result{
				{Entity: search.EntityPerson, ID: 1, Title: "Ann Smith", Snippet: template.HTML("<mark>Ann</mark> Smith is a friend")},
			},
		},
		{
			Entity: search.EntityEvent,
			Results: []search.Result{
				{Entity: search.EntityEvent, ID: 2, Title: "birthday", ParentTitle: "Ann Smith", Snippet: template.HTML("birthday on <mark>May 1</mark> &amp; more")},
			},
		},
	}
}

func TestSearch_Groups(t *testing.T) {
	b, fs := newTestBot(nil, nil, nil)
	fsearch := &fakeSearcher{groups: searchGroups()}
	b.SetSearcher(fsearch)

	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/search ann"}})
	msg := fs.last()
	if fsearch.gotQuery != "ann" {
		t.Fatalf("expected trimmed query, got %q", fsearch.gotQuery)
	}
	for _, want := range []string{"People", "Ann Smith", "Events", "birthday", "May 1", "& more"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("search reply missing %q: %q", want, msg)
		}
	}
	if strings.Contains(msg, "<mark>") || strings.Contains(msg, "</mark>") {
		t.Fatalf("snippet markup leaked into plain-text reply: %q", msg)
	}
	if strings.Contains(msg, "Ann Smith · Ann Smith") {
		t.Fatalf("duplicate owner/title label: %q", msg)
	}
}

func TestSearch_Unavailable(t *testing.T) {
	// No searcher wired at all.
	b, fs := newTestBot(nil, nil, nil)
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/search ann"}})
	if !strings.Contains(fs.last(), "unavailable") {
		t.Fatalf("expected unavailable reply, got %q", fs.last())
	}

	// Index missing behind the searcher.
	b2, fs2 := newTestBot(nil, nil, nil)
	b2.SetSearcher(&fakeSearcher{err: fmt.Errorf("%w: no such table", search.ErrIndexUnavailable)})
	b2.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/search ann"}})
	if !strings.Contains(fs2.last(), "unavailable") {
		t.Fatalf("expected unavailable reply, got %q", fs2.last())
	}
	if strings.Contains(fs2.last(), "no such table") {
		t.Fatalf("driver detail must not leak: %q", fs2.last())
	}
}

func TestSearch_NoMatches(t *testing.T) {
	b, fs := newTestBot(nil, nil, nil)
	b.SetSearcher(&fakeSearcher{groups: nil})
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/search zzz"}})
	if !strings.Contains(fs.last(), "No matches") {
		t.Fatalf("expected no matches, got %q", fs.last())
	}

	// Generic error also degrades to unavailable rather than panicking.
	b2, fs2 := newTestBot(nil, nil, nil)
	b2.SetSearcher(&fakeSearcher{err: errors.New("boom")})
	b2.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/search zzz"}})
	if !strings.Contains(fs2.last(), "unavailable") {
		t.Fatalf("expected unavailable, got %q", fs2.last())
	}
}

func TestSearch_Truncation(t *testing.T) {
	longTitle := strings.Repeat("Long owner name ", 12)
	var groups []search.Group
	for g := 0; g < maxSearchGroups; g++ {
		group := search.Group{Entity: search.EntityPersonNote}
		for i := 0; i < maxSearchPerGroup*2; i++ {
			group.Results = append(group.Results, search.Result{
				Entity:      search.EntityPersonNote,
				ID:          i,
				ParentTitle: longTitle,
				Snippet:     template.HTML(strings.Repeat("very long snippet text ", 40)),
			})
		}
		groups = append(groups, group)
	}
	b, fs := newTestBot(nil, nil, nil)
	b.SetSearcher(&fakeSearcher{groups: groups})
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/search person"}})
	msg := fs.last()
	if len([]rune(msg)) > maxMessageRunes+len([]rune(truncationMarker)) {
		t.Fatalf("reply too long: %d runes", len([]rune(msg)))
	}
	if !strings.Contains(msg, truncationMarker) {
		t.Fatalf("expected truncation marker, got %q", msg[:min(200, len(msg))])
	}
	if !strings.Contains(msg, "and 5 more") {
		t.Fatalf("expected per-group overflow marker, got %q", msg[:min(200, len(msg))])
	}
}

func TestSearch_MissingArg(t *testing.T) {
	b, fs := newTestBot(nil, nil, nil)
	b.SetSearcher(&fakeSearcher{groups: searchGroups()})
	b.HandleUpdate(context.Background(), &Update{Message: &Message{Chat: &Chat{ID: 1}, Text: "/search"}})
	if !strings.Contains(fs.last(), "Usage: /search") {
		t.Fatalf("expected usage, got %q", fs.last())
	}
}

func TestFormatLine_Milestone(t *testing.T) {
	e := mkEvent("Ann", "birthday", time.Date(1996, 1, 1, 0, 0, 0, 0, time.UTC))
	occDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	todayStart := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	line := formatLine(e, occDate, todayStart)
	if !strings.Contains(line, "30th birthday") {
		t.Fatalf("expected milestone, got %q", line)
	}
}
