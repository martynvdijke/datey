package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	_ "github.com/mattn/go-sqlite3"
)

var settingsNotifDBSeq atomic.Int64

// newNotificationSettingsHandler builds a handler on its own in-memory
// database and seeds the signed-in user (ID 1, matching withUserContext).
func newNotificationSettingsHandler(t *testing.T) *Handler {
	t.Helper()
	dsn := fmt.Sprintf("file:test_settings_notifications_%d?mode=memory&cache=shared&_fk=1", settingsNotifDBSeq.Add(1))
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if _, err := client.User.Create().
		SetUsername("testuser").
		SetPasswordHash("hash").
		SetRole(user.RoleAdmin).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return NewHandler(&config.Config{ReminderDays: 7}, client, notifier.NewRegistry(), logstore.NewStore(100))
}

func renderNotificationSettings(t *testing.T, h *Handler) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/settings/notifications", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.settingsNotifications(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestSettingsNotifications_RendersChannelCards(t *testing.T) {
	h := newNotificationSettingsHandler(t)
	body := renderNotificationSettings(t, h)

	for _, channel := range notificationChannelOrder {
		if !strings.Contains(body, `data-channel="`+channel+`"`) {
			t.Errorf("missing card for channel %q", channel)
		}
	}
	if !strings.Contains(body, `name="target_email"`) {
		t.Error("missing email target input")
	}
	if !strings.Contains(body, `name="enabled_webpush"`) {
		t.Error("missing webpush enable checkbox")
	}
}

func TestSettingsNotifications_HealthPresent(t *testing.T) {
	h := newNotificationSettingsHandler(t)
	ctx := context.Background()

	if _, err := h.client.UserNotificationChannel.Create().
		SetUserID(1).
		SetChannelType("telegram").
		SetTarget("chat-99").
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	success := time.Date(2026, 11, 10, 7, 30, 0, 0, time.UTC)
	failure := time.Date(2026, 11, 11, 6, 15, 0, 0, time.UTC)
	if _, err := h.client.ChannelHealth.Create().
		SetChannel("telegram").
		SetTarget("chat-99").
		SetLastSuccessAt(success).
		SetLastFailureAt(failure).
		SetConsecutiveFailures(2).
		SetLastError("telegram returned status 502").
		Save(ctx); err != nil {
		t.Fatalf("seed health: %v", err)
	}

	body := renderNotificationSettings(t, h)
	for _, want := range []string{
		`data-channel-health="telegram"`,
		success.Local().Format(channelHealthTimeLayout),
		failure.Local().Format(channelHealthTimeLayout),
		"telegram returned status 502",
		`role="alert"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	if !strings.Contains(body, ">2<") {
		t.Errorf("body missing consecutive failure count 2")
	}
}

func TestSettingsNotifications_HealthAbsentShowsHint(t *testing.T) {
	h := newNotificationSettingsHandler(t)
	body := renderNotificationSettings(t, h)
	if strings.Contains(body, "data-channel-health=") {
		t.Error("body rendered a health block without any health row")
	}
	if !strings.Contains(body, "data-channel-health-empty") {
		t.Error("missing never-attempted hint")
	}
}

func TestSettingsNotifications_SuccessOnlyIsNotFailure(t *testing.T) {
	h := newNotificationSettingsHandler(t)
	ctx := context.Background()
	if _, err := h.client.ChannelHealth.Create().
		SetChannel("email").
		SetTarget("").
		SetLastSuccessAt(time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)).
		SetConsecutiveFailures(0).
		Save(ctx); err != nil {
		t.Fatalf("seed health: %v", err)
	}
	body := renderNotificationSettings(t, h)
	if strings.Contains(body, `role="alert"`) {
		t.Error("a channel with only successes rendered a failure alert")
	}
	if !strings.Contains(body, `role="status"`) {
		t.Error("missing healthy delivery status")
	}
}

// TestSettingsNotifications_BatchesHealthQuery guards against N+1 queries: the
// page loads every health row once, no matter how many channels exist.
func TestSettingsNotifications_BatchesHealthQuery(t *testing.T) {
	h := newNotificationSettingsHandler(t)
	ctx := context.Background()
	for _, channel := range notificationChannelOrder {
		if _, err := h.client.UserNotificationChannel.Create().
			SetUserID(1).
			SetChannelType(channel).
			SetTarget("target-" + channel).
			SetEnabled(true).
			Save(ctx); err != nil {
			t.Fatalf("seed channel %s: %v", channel, err)
		}
		if _, err := h.client.ChannelHealth.Create().
			SetChannel(channel).
			SetTarget("target-" + channel).
			SetConsecutiveFailures(1).
			SetLastError("down").
			Save(ctx); err != nil {
			t.Fatalf("seed health %s: %v", channel, err)
		}
	}

	var healthQueries int
	h.client.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			if _, ok := query.(*ent.ChannelHealthQuery); ok {
				healthQueries++
			}
			return next.Query(ctx, query)
		})
	}))

	body := renderNotificationSettings(t, h)
	if healthQueries != 1 {
		t.Errorf("ChannelHealth queries = %d, want 1 (N+1 regression)", healthQueries)
	}
	for _, channel := range notificationChannelOrder {
		if !strings.Contains(body, `data-channel-health="`+channel+`"`) {
			t.Errorf("missing health block for %q", channel)
		}
	}
}

func TestBuildChannelCards_NoHealthWithoutConfiguredTargetMatch(t *testing.T) {
	channels := map[string]*ent.UserNotificationChannel{
		"ntfy": {ChannelType: "ntfy", Target: "new-topic", Enabled: true},
	}
	health := map[string]*ent.ChannelHealth{
		"ntfy\x1fold-topic": {Channel: "ntfy", Target: "old-topic", ConsecutiveFailures: 3, LastError: "stale"},
	}
	cards := buildChannelCards(channels, health)
	for _, card := range cards {
		if card.Channel != "ntfy" {
			continue
		}
		if card.HasHealth {
			t.Error("card picked up health from a different target")
		}
		if card.Target != "new-topic" || !card.Enabled {
			t.Errorf("card = %+v", card)
		}
	}
}
