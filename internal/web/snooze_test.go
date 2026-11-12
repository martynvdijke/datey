package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/reminderaction"
	"github.com/go-chi/chi/v5"
)

const actionTestSecret = "action-test-feed-key"

var actionTestOccasion = time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)

func newActionHandler(t *testing.T) *Handler {
	t.Helper()
	h := newTestWebHandler(t)
	h.cfg.ICalFeedKey = actionTestSecret
	return h
}

func actionToken(t *testing.T, eventID int, occasion time.Time, userID int, action string, ttl time.Duration) string {
	t.Helper()
	token, err := reminderaction.Mint([]byte(actionTestSecret), eventID, occasion, userID, action, ttl)
	if err != nil {
		t.Fatalf("mint action token: %v", err)
	}
	return token
}

func setupActionRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/a/{token}", h.handleReminderAction)
	r.Post("/a/{token}", h.handleReminderActionSubmit)
	return r
}

func doAction(t *testing.T, h *Handler, method, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, "/a/"+token, nil)
	} else {
		req = httptest.NewRequest(method, "/a/"+token, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rr := httptest.NewRecorder()
	setupActionRouter(h).ServeHTTP(rr, req)
	return rr
}

func snoozeRows(t *testing.T, h *Handler) []*ent.Snooze {
	t.Helper()
	all, err := h.client.Snooze.Query().All(context.Background())
	if err != nil {
		t.Fatalf("query snoozes: %v", err)
	}
	return all
}

func TestReminderActionGetRendersConfirmation(t *testing.T) {
	h := newActionHandler(t)

	for _, action := range []string{reminderaction.ActionAck, reminderaction.ActionSnooze} {
		token := actionToken(t, 4, actionTestOccasion, 0, action, time.Hour)
		rr := doAction(t, h, http.MethodGet, token, "")
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", action, rr.Code)
		}
		if got := rr.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Errorf("%s: Cache-Control = %q, want no-store", action, got)
		}
		if got := rr.Header().Get("X-Robots-Tag"); !strings.Contains(got, "noindex") {
			t.Errorf("%s: X-Robots-Tag = %q, want noindex", action, got)
		}
		body := rr.Body.String()
		if !strings.Contains(body, `data-reminder-action="`+action+`"`) {
			t.Errorf("%s: body does not carry the action marker", action)
		}
		if !strings.Contains(body, "2026") {
			t.Errorf("%s: body does not show the occasion date: %s", action, body)
		}
	}
}

func TestReminderActionGetRejectsBadTokensWith404(t *testing.T) {
	h := newActionHandler(t)
	valid := actionToken(t, 4, actionTestOccasion, 0, reminderaction.ActionAck, time.Hour)
	// Tamper inside a base64 group, not at the end: the final character of an
	// unpadded 32-byte signature carries only two significant bits, so flipping
	// it can decode to the very same bytes and the token stays valid.
	tamperedBytes := []byte(valid)
	tamperedBytes[5] = 'A'
	if valid[5] == 'A' {
		tamperedBytes[5] = 'B'
	}
	tampered := string(tamperedBytes)

	cases := map[string]string{
		"tampered":  tampered,
		"garbage":   "not-a-token",
		"empty-ish": "..",
	}
	for name, token := range cases {
		rr := doAction(t, h, http.MethodGet, token, "")
		if rr.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", name, rr.Code)
		}
		if got := rr.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Errorf("%s: 404 response is cacheable: %q", name, got)
		}
	}

	// A token signed with a different key must be a 404 too.
	other, err := reminderaction.Mint([]byte("other-key"), 4, actionTestOccasion, 0, reminderaction.ActionAck, time.Hour)
	if err != nil {
		t.Fatalf("mint other-key token: %v", err)
	}
	if rr := doAction(t, h, http.MethodGet, other, ""); rr.Code != http.StatusNotFound {
		t.Errorf("foreign signature: status = %d, want 404", rr.Code)
	}
}

func TestReminderActionPostAckRecordsMarker(t *testing.T) {
	h := newActionHandler(t)
	token := actionToken(t, 11, actionTestOccasion, 3, reminderaction.ActionAck, time.Hour)

	rr := doAction(t, h, http.MethodPost, token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `data-reminder-action="ack"`) {
		t.Error("done page does not carry the action marker")
	}

	active, err := h.snoozes().Active(context.Background(), 11, actionTestOccasion, 3, time.Now())
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	if !active {
		t.Fatal("Active() = false after ack")
	}
}

func TestReminderActionPostIsIdempotent(t *testing.T) {
	h := newActionHandler(t)
	token := actionToken(t, 12, actionTestOccasion, 0, reminderaction.ActionSnooze, time.Hour)

	for i := 0; i < 3; i++ {
		rr := doAction(t, h, http.MethodPost, token, "days=4")
		if rr.Code != http.StatusOK {
			t.Fatalf("post %d: status = %d, want 200", i, rr.Code)
		}
	}
	if rows := snoozeRows(t, h); len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 after repeated posts", len(rows))
	}
}

func TestReminderActionPostSnoozeClampsDays(t *testing.T) {
	cases := map[string]int{
		"":         3,
		"days=5":   5,
		"days=0":   1,
		"days=99":  30,
		"days=abc": 3,
	}
	for body, wantDays := range cases {
		t.Run("body "+body, func(t *testing.T) {
			h := newActionHandler(t)
			token := actionToken(t, 13, actionTestOccasion, 0, reminderaction.ActionSnooze, time.Hour)
			rr := doAction(t, h, http.MethodPost, token, body)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			rows := snoozeRows(t, h)
			if len(rows) != 1 {
				t.Fatalf("rows = %d, want 1", len(rows))
			}
			want := time.Duration(wantDays) * 24 * time.Hour
			if diff := time.Until(rows[0].SnoozedUntil) - want; diff < -2*time.Hour || diff > 2*time.Hour {
				t.Errorf("snoozed for %v, want about %v", time.Until(rows[0].SnoozedUntil), want)
			}
		})
	}
}

func TestReminderActionPostRejectsBadTokenWithoutWriting(t *testing.T) {
	h := newActionHandler(t)
	other, err := reminderaction.Mint([]byte("other-key"), 14, actionTestOccasion, 0, reminderaction.ActionAck, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if rr := doAction(t, h, http.MethodPost, other, ""); rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	if rows := snoozeRows(t, h); len(rows) != 0 {
		t.Errorf("rows = %d, want 0 after a rejected token", len(rows))
	}
}

func TestActionSecretOrder(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.ICalFeedKey = ""
	h.cfg.UpcomingAPIKey = "upcoming-key"
	h.cfg.RSSFeedKey = "rss-key"
	if got := string(h.actionSecret()); got != "upcoming-key" {
		t.Errorf("actionSecret() = %q, want the upcoming key", got)
	}
	h.cfg.UpcomingAPIKey = ""
	if got := string(h.actionSecret()); got != "rss-key" {
		t.Errorf("actionSecret() = %q, want the rss key", got)
	}
	h.cfg.RSSFeedKey = ""
	if got := h.actionSecret(); got != nil {
		t.Errorf("actionSecret() = %q, want nil when no feed key is configured", got)
	}
}
