package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/oauth2"
)

func newTestFCMNotifier(t *testing.T, cfg *config.Config) (*FCMNotifier, *repository.PushSubscriptionRepository, *ent.Client) {
	t.Helper()
	dsn := fmt.Sprintf("file:test_fcm_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { client.Close() })
	subs := repository.NewPushSubscriptionRepository(client)
	n := NewFCMNotifier(cfg, subs)
	// Bypass service-account auth: a static token exercises the same code path
	// as a cached OAuth token source.
	n.tokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-access-token"})
	return n, subs, client
}

func seedFCMUser(t *testing.T, client *ent.Client) *ent.User {
	t.Helper()
	u, err := client.User.Create().
		SetUsername(fmt.Sprintf("fcm-user-%d", time.Now().UnixNano())).
		SetPasswordHash("hash").
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func seedFCMDevice(t *testing.T, subs *repository.PushSubscriptionRepository, userID int, token string) {
	t.Helper()
	if _, err := subs.Upsert(context.Background(), userID, "fcm:"+token, "fcm", "fcm"); err != nil {
		t.Fatalf("seed device: %v", err)
	}
}

func fcmTestConfig() *config.Config {
	return &config.Config{
		FCMEnabled:            true,
		FCMProjectID:          "datey-test",
		FCMServiceAccountJSON: `{"type":"service_account"}`,
	}
}

func TestFCMNotifier_Name(t *testing.T) {
	n, _, _ := newTestFCMNotifier(t, &config.Config{})
	if got := n.Name(); got != "fcm" {
		t.Errorf("Name() = %q, want %q", got, "fcm")
	}
}

func TestFCMNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"disabled", &config.Config{FCMEnabled: false, FCMProjectID: "p", FCMServiceAccountJSON: "{}"}, false},
		{"enabled no project", &config.Config{FCMEnabled: true, FCMServiceAccountJSON: "{}"}, false},
		{"enabled no credentials", &config.Config{FCMEnabled: true, FCMProjectID: "p"}, false},
		{"enabled complete", &config.Config{FCMEnabled: true, FCMProjectID: "p", FCMServiceAccountJSON: "{}"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, _, _ := newTestFCMNotifier(t, tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFCMNotifier_SendToUser_PayloadAndBearer(t *testing.T) {
	type captured struct {
		method string
		path   string
		auth   string
		body   fcmPayload
	}
	got := make(chan captured, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body fcmPayload
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		got <- captured{method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"projects/datey-test/messages/1"}`))
	}))
	defer srv.Close()

	n, subs, client := newTestFCMNotifier(t, fcmTestConfig())
	n.baseURL = srv.URL
	u := seedFCMUser(t, client)
	seedFCMDevice(t, subs, u.ID, "device-token-1")

	if err := n.SendToUser(context.Background(), u.ID, "New person", "Dana added", "/people/7", "person"); err != nil {
		t.Fatalf("SendToUser() error = %v", err)
	}

	c := <-got
	if c.method != http.MethodPost {
		t.Errorf("method = %q, want POST", c.method)
	}
	if want := "/v1/projects/datey-test/messages:send"; c.path != want {
		t.Errorf("path = %q, want %q", c.path, want)
	}
	if c.auth != "Bearer test-access-token" {
		t.Errorf("Authorization = %q, want %q", c.auth, "Bearer test-access-token")
	}
	if c.body.Message.Token != "device-token-1" {
		t.Errorf("token = %q", c.body.Message.Token)
	}
	if c.body.Message.Notification.Title != "New person" || c.body.Message.Notification.Body != "Dana added" {
		t.Errorf("notification = %+v", c.body.Message.Notification)
	}
	if c.body.Message.Data["url"] != "/people/7" || c.body.Message.Data["kind"] != "person" {
		t.Errorf("data = %+v", c.body.Message.Data)
	}
	if c.body.Message.Android == nil {
		t.Fatal("android config missing")
	}
	if c.body.Message.Android.Priority != "high" {
		t.Errorf("android priority = %q, want high", c.body.Message.Android.Priority)
	}
	if c.body.Message.Android.Notification == nil || c.body.Message.Android.Notification.ChannelID != "datey_new_items" {
		t.Errorf("android notification = %+v", c.body.Message.Android.Notification)
	}
}

func TestFCMNotifier_SendToUser_OnlyOwnTokens(t *testing.T) {
	var mu sync.Mutex
	var tokens []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body fcmPayload
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		tokens = append(tokens, body.Message.Token)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n, subs, client := newTestFCMNotifier(t, fcmTestConfig())
	n.baseURL = srv.URL
	owner := seedFCMUser(t, client)
	other := seedFCMUser(t, client)
	seedFCMDevice(t, subs, owner.ID, "owner-token")
	seedFCMDevice(t, subs, other.ID, "other-token")

	if err := n.SendToUser(context.Background(), owner.ID, "t", "m", "/people/1", "person"); err != nil {
		t.Fatalf("SendToUser() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(tokens) != 1 || tokens[0] != "owner-token" {
		t.Errorf("tokens = %v, want [owner-token]", tokens)
	}
}

func TestFCMNotifier_SendToUser_PrunesUnregistered(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "UNREGISTERED",
			body: `{"error":{"code":404,"message":"Requested entity was not found.","status":"NOT_FOUND","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`,
		},
		{
			name: "INVALID_ARGUMENT",
			body: `{"error":{"code":400,"message":"Invalid token","status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"INVALID_ARGUMENT"}]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			n, subs, client := newTestFCMNotifier(t, fcmTestConfig())
			n.baseURL = srv.URL
			u := seedFCMUser(t, client)
			seedFCMDevice(t, subs, u.ID, "dead-token")

			if err := n.SendToUser(context.Background(), u.ID, "t", "m", "/people/1", "person"); err == nil {
				t.Errorf("expected an error for a rejected token")
			}
			count, err := subs.Count(context.Background())
			if err != nil {
				t.Fatalf("Count: %v", err)
			}
			if count != 0 {
				t.Errorf("expected token to be pruned, count = %d", count)
			}
		})
	}
}

func TestFCMNotifier_SendToUser_KeepsTokenOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":500,"message":"boom","status":"INTERNAL"}}`))
	}))
	defer srv.Close()

	n, subs, client := newTestFCMNotifier(t, fcmTestConfig())
	n.baseURL = srv.URL
	u := seedFCMUser(t, client)
	seedFCMDevice(t, subs, u.ID, "live-token")

	if err := n.SendToUser(context.Background(), u.ID, "t", "m", "/people/1", "person"); err == nil {
		t.Errorf("expected an error from a 500 response")
	}
	count, err := subs.Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Errorf("a transient failure must not prune the token, count = %d", count)
	}
}
