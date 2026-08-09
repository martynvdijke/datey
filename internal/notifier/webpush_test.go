package notifier

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/SherClockHolmes/webpush-go"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

// testWebPushKeys returns a valid VAPID key pair (base64url, PKCS8/SEC1 EC).
func testWebPushKeys(t *testing.T) (privateKey, publicKey string) {
	t.Helper()
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	return priv, pub
}

// testSubscriptionKeys returns base64url p256dh (valid uncompressed EC point)
// and auth (16 bytes) that webpush-go can actually encrypt to.
func testSubscriptionKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	point, err := key.PublicKey.Bytes()
	if err != nil {
		t.Fatalf("PublicKey.Bytes: %v", err)
	}
	if len(point) != 65 || point[0] != 0x04 {
		t.Fatalf("unexpected uncompressed point length: %d", len(point))
	}
	authBytes := make([]byte, 16)
	if _, err := rand.Read(authBytes); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(point), base64.RawURLEncoding.EncodeToString(authBytes)
}

func newTestWebPushNotifier(t *testing.T, cfg *config.Config) (*WebPushNotifier, *repository.PushSubscriptionRepository, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_webpush?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	subs := repository.NewPushSubscriptionRepository(client)
	return NewWebPushNotifier(cfg, subs), subs, client
}

func seedWebPushSubscription(t *testing.T, client *ent.Client, subs *repository.PushSubscriptionRepository, endpoint, p256dh, auth string) {
	t.Helper()
	ctx := context.Background()
	u, err := client.User.Create().
		SetUsername("push-user").
		SetPasswordHash("hash").
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := subs.Upsert(ctx, u.ID, endpoint, p256dh, auth); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
}

func TestWebPushNotifier_Name(t *testing.T) {
	n, _, _ := newTestWebPushNotifier(t, &config.Config{})
	if got := n.Name(); got != "webpush" {
		t.Errorf("Name() = %q, want %q", got, "webpush")
	}
}

func TestWebPushNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"disabled", &config.Config{PushEnabled: false, PushVAPIDPublicKey: "pk", PushVAPIDPrivateKey: "sk"}, false},
		{"enabled no keys", &config.Config{PushEnabled: true}, false},
		{"enabled with keys", &config.Config{PushEnabled: true, PushVAPIDPublicKey: "pk", PushVAPIDPrivateKey: "sk"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, _, _ := newTestWebPushNotifier(t, tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebPushNotifier_Send_Success(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	priv, pub := testWebPushKeys(t)
	cfg := &config.Config{PushEnabled: true, PushVAPIDPublicKey: pub, PushVAPIDPrivateKey: priv}
	n, subs, client := newTestWebPushNotifier(t, cfg)
	p256dh, auth := testSubscriptionKeys(t)
	seedWebPushSubscription(t, client, subs, srv.URL, p256dh, auth)

	if err := n.Send(context.Background(), "Reminder: Dana - birthday", "Dana's birthday is in 3 days"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/" {
		t.Errorf("path = %q, want %q", gotPath, "/")
	}
}

func TestWebPushNotifier_Send_PrunesGoneEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	priv, pub := testWebPushKeys(t)
	cfg := &config.Config{PushEnabled: true, PushVAPIDPublicKey: pub, PushVAPIDPrivateKey: priv}
	n, subs, client := newTestWebPushNotifier(t, cfg)
	p256dh, auth := testSubscriptionKeys(t)
	seedWebPushSubscription(t, client, subs, srv.URL, p256dh, auth)

	if err := n.Send(context.Background(), "t", "m"); err != nil {
		t.Fatalf("Send() with gone endpoint: %v", err)
	}
	count, err := subs.Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected pruned subscription, count = %d", count)
	}
}

func TestWebPushNotifier_Send_ErrorPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	priv, pub := testWebPushKeys(t)
	cfg := &config.Config{PushEnabled: true, PushVAPIDPublicKey: pub, PushVAPIDPrivateKey: priv}
	n, subs, client := newTestWebPushNotifier(t, cfg)
	p256dh, auth := testSubscriptionKeys(t)
	seedWebPushSubscription(t, client, subs, srv.URL, p256dh, auth)

	if err := n.Send(context.Background(), "t", "m"); err == nil {
		t.Errorf("expected error from 500 response")
	}
}
