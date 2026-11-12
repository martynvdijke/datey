package icsub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/ics"
	"github.com/datey/datey/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

func newTestClient(t *testing.T) *ent.Client {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(strings.ReplaceAll(t.Name(), "/", "_"), " ", "_") + "?mode=memory&cache=shared&_fk=1"
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { client.Close() })
	return client
}

type fakeFetcher struct {
	events []ics.Event
	err    error
}

func (f *fakeFetcher) Fetch(context.Context, string) ([]ics.Event, error) {
	return f.events, f.err
}

func newSubscription(t *testing.T, client *ent.Client) *ent.Subscription {
	t.Helper()
	sub, err := repository.NewSubscriptionRepository(client).Create(context.Background(), "School", "https://example.com/feed.ics", true)
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	return sub
}

func TestSync_Diff(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, time.August, d, 0, 0, 0, 0, time.UTC) }
	ev := func(uid, summary string, d int) ics.Event {
		return ics.Event{UID: uid, Summary: summary, Start: day(d), AllDay: true}
	}

	tests := []struct {
		name    string
		local   []ics.Event
		feed    []ics.Event
		want    Summary
		remain  int
		wantSum string // summary of the single remaining event; "" = not checked
	}{
		{
			name:   "adds new events",
			feed:   []ics.Event{ev("a", "Alpha", 1), ev("b", "Beta", 2)},
			want:   Summary{Created: 2},
			remain: 2,
		},
		{
			name:   "no-op when unchanged",
			local:  []ics.Event{ev("a", "Alpha", 1)},
			feed:   []ics.Event{ev("a", "Alpha", 1)},
			want:   Summary{},
			remain: 1,
		},
		{
			name:    "updates changed summary",
			local:   []ics.Event{ev("a", "Old", 1)},
			feed:    []ics.Event{ev("a", "New", 1)},
			want:    Summary{Updated: 1},
			remain:  1,
			wantSum: "New",
		},
		{
			name:    "updates changed date",
			local:   []ics.Event{ev("a", "Alpha", 1)},
			feed:    []ics.Event{ev("a", "Alpha", 9)},
			want:    Summary{Updated: 1},
			remain:  1,
			wantSum: "Alpha",
		},
		{
			name:    "removes vanished events",
			local:   []ics.Event{ev("a", "Alpha", 1), ev("b", "Beta", 2)},
			feed:    []ics.Event{ev("a", "Alpha", 1)},
			want:    Summary{Deleted: 1},
			remain:  1,
			wantSum: "Alpha",
		},
		{
			name:   "uid-less feed never deletes",
			local:  []ics.Event{ev("a", "Alpha", 1)},
			feed:   []ics.Event{ev("", "No uid", 5)},
			want:   Summary{},
			remain: 1,
		},
		{
			name:   "empty feed never deletes",
			local:  []ics.Event{ev("a", "Alpha", 1)},
			feed:   nil,
			want:   Summary{},
			remain: 1,
		},
		{
			name:    "duplicate uids keep the first",
			feed:    []ics.Event{ev("dup", "First", 1), ev("dup", "Second", 2)},
			want:    Summary{Created: 1},
			remain:  1,
			wantSum: "First",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t)
			sub := newSubscription(t, client)
			ctx := context.Background()
			events := repository.NewEventRepository(client)
			for _, le := range tc.local {
				if _, err := events.CreateForSubscription(ctx, sub.ID, le.UID, eventType, le.Start, le.Summary); err != nil {
					t.Fatalf("seed local event: %v", err)
				}
			}

			syncer := NewSyncer(client, &fakeFetcher{events: tc.feed})
			got, err := syncer.Sync(ctx, sub)
			if err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if got != tc.want {
				t.Fatalf("summary = %+v, want %+v", got, tc.want)
			}

			remaining, err := repository.NewSubscriptionRepository(client).EventsBySubscription(ctx, sub.ID)
			if err != nil {
				t.Fatalf("list events: %v", err)
			}
			if len(remaining) != tc.remain {
				t.Fatalf("remaining events = %d, want %d", len(remaining), tc.remain)
			}
			if tc.wantSum != "" && remaining[0].Description != tc.wantSum {
				t.Errorf("remaining summary = %q, want %q", remaining[0].Description, tc.wantSum)
			}
		})
	}
}

func TestSync_FetchFailureLeavesEventsUntouched(t *testing.T) {
	client := newTestClient(t)
	sub := newSubscription(t, client)
	ctx := context.Background()
	events := repository.NewEventRepository(client)
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForSubscription(ctx, sub.ID, "a", eventType, start, "Alpha"); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	syncer := NewSyncer(client, &fakeFetcher{err: errors.New("provider down")})
	summary, err := syncer.Sync(ctx, sub)
	if err == nil {
		t.Fatal("want error for failed fetch, got nil")
	}
	if summary != (Summary{}) {
		t.Errorf("summary = %+v, want zero", summary)
	}

	remaining, err := repository.NewSubscriptionRepository(client).EventsBySubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("events changed on failed fetch: got %d, want 1", len(remaining))
	}

	got, err := repository.NewSubscriptionRepository(client).Get(ctx, sub.ID)
	if err != nil {
		t.Fatalf("reload subscription: %v", err)
	}
	if got.LastAttemptAt == nil {
		t.Error("last_attempt_at not recorded")
	}
	if got.LastSuccessAt != nil {
		t.Error("last_success_at must stay unset on failure")
	}
	if !strings.Contains(got.LastError, "provider down") {
		t.Errorf("last_error = %q, want it to mention the failure", got.LastError)
	}
}

func TestSync_RecordsSuccess(t *testing.T) {
	client := newTestClient(t)
	sub := newSubscription(t, client)
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

	syncer := NewSyncer(client, &fakeFetcher{events: []ics.Event{{UID: "a", Summary: "Alpha", Start: start}}})
	if _, err := syncer.Sync(context.Background(), sub); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	got, err := repository.NewSubscriptionRepository(client).Get(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("reload subscription: %v", err)
	}
	if got.LastAttemptAt == nil || got.LastSuccessAt == nil {
		t.Fatalf("timestamps not recorded: attempt=%v success=%v", got.LastAttemptAt, got.LastSuccessAt)
	}
	if got.LastError != "" {
		t.Errorf("last_error = %q, want empty after success", got.LastError)
	}
}

func TestSync_DoesNotTouchManualEvents(t *testing.T) {
	client := newTestClient(t)
	sub := newSubscription(t, client)
	ctx := context.Background()
	events := repository.NewEventRepository(client)
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

	// A manual event: no subscription, no external UID.
	manual, err := client.Event.Create().
		SetType("custom").
		SetDate(start).
		SetDescription("Manual").
		SetCreatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed manual event: %v", err)
	}

	syncer := NewSyncer(client, &fakeFetcher{events: []ics.Event{{UID: "a", Summary: "Alpha", Start: start}}})
	if _, err := syncer.Sync(ctx, sub); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if _, err := events.Get(ctx, manual.ID); err != nil {
		t.Fatalf("manual event was removed: %v", err)
	}
}

func TestHTTPFetcher(t *testing.T) {
	const feed = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:abc\r\nSUMMARY:Hello\r\nDTSTART;VALUE=DATE:20260812\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantErr     bool
	}{
		{name: "valid", status: http.StatusOK, contentType: "text/calendar", body: feed},
		{name: "lenient content type", status: http.StatusOK, contentType: "text/plain", body: feed},
		{name: "html rejected", status: http.StatusOK, contentType: "text/html", body: "<html>nope</html>", wantErr: true},
		{name: "non 2xx", status: http.StatusNotFound, contentType: "text/calendar", body: feed, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			got, err := NewHTTPFetcher().Fetch(context.Background(), srv.URL)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if len(got) != 1 || got[0].UID != "abc" || got[0].Summary != "Hello" {
				t.Fatalf("unexpected events: %+v", got)
			}
		})
	}
}

func TestHTTPFetcher_RejectsUnsupportedScheme(t *testing.T) {
	if _, err := NewHTTPFetcher().Fetch(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatal("want error for non-HTTP URL, got nil")
	}
}
