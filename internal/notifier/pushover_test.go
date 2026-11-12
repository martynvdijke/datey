package notifier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/datey/datey/internal/config"
)

func firstFormValue(form map[string][]string, key string) string {
	if values, ok := form[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

func TestPushoverNotifier_Name(t *testing.T) {
	n := NewPushoverNotifier(&config.Config{})
	if got := n.Name(); got != "pushover" {
		t.Errorf("Name() = %q, want %q", got, "pushover")
	}
}

func TestPushoverNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"token and user", &config.Config{PushoverToken: "app-token", PushoverUser: "user-key"}, true},
		{"token only (per-user targets may supply a key)", &config.Config{PushoverToken: "app-token"}, true},
		{"missing token", &config.Config{PushoverUser: "user-key"}, false},
		{"token whitespace", &config.Config{PushoverToken: "   "}, false},
		{"both empty", &config.Config{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewPushoverNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPushoverNotifier_Send_FormFields(t *testing.T) {
	var mu sync.Mutex
	var gotPath, gotMethod, gotContentType string
	var gotForm map[string][]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewPushoverNotifier(&config.Config{
		PushoverToken: "app-token-123",
		PushoverUser:  "user-key-456",
	})
	n.endpoint = srv.URL

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
	if ct := gotContentType; !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
	}

	want := map[string]string{
		"token":    "app-token-123",
		"user":     "user-key-456",
		"message":  "Dana's birthday is in 3 days",
		"title":    "Reminder: Dana - birthday",
		"priority": "0",
	}
	for field, value := range want {
		if got := firstFormValue(gotForm, field); got != value {
			t.Errorf("form %s = %q, want %q", field, got, value)
		}
	}
	if got := firstFormValue(gotForm, "url"); got != "" {
		t.Errorf("form url = %q, want it omitted", got)
	}
	if got := firstFormValue(gotForm, "url_title"); got != "" {
		t.Errorf("form url_title = %q, want it omitted", got)
	}
}

func TestPushoverNotifier_SendTo_PerUserTargetOverridesDefault(t *testing.T) {
	var mu sync.Mutex
	var gotUser string
	var calls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		mu.Lock()
		defer mu.Unlock()
		calls++
		gotUser = r.PostForm.Get("user")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.Config{PushoverToken: "app-token", PushoverUser: "default-user"}
	n := NewPushoverNotifier(cfg)
	n.endpoint = srv.URL

	if err := n.SendTo(context.Background(), "t", "m", "override-user"); err != nil {
		t.Fatalf("SendTo() error = %v", err)
	}
	mu.Lock()
	override := gotUser
	mu.Unlock()
	if override != "override-user" {
		t.Errorf("user = %q, want %q", override, "override-user")
	}

	// An empty target must fall back to the configured default.
	if err := n.SendTo(context.Background(), "t", "m", ""); err != nil {
		t.Fatalf("SendTo() with empty target error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotUser != "default-user" {
		t.Errorf("user = %q, want default %q", gotUser, "default-user")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestPushoverNotifier_Send_MissingAppToken(t *testing.T) {
	n := NewPushoverNotifier(&config.Config{PushoverUser: "user-key"})
	err := n.Send(context.Background(), "t", "m")
	if err == nil {
		t.Fatal("expected error when app token is missing, got nil")
	}
	if !strings.Contains(err.Error(), "application token not configured") {
		t.Errorf("error = %q, want it to mention the missing application token", err)
	}
}

func TestPushoverNotifier_Send_Non2xxSurfacesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":0,"request":"abc123","errors":["user key is invalid"]}`))
	}))
	defer srv.Close()

	n := NewPushoverNotifier(&config.Config{PushoverToken: "app-token", PushoverUser: "user-key"})
	n.endpoint = srv.URL

	err := n.Send(context.Background(), "t", "m")
	if err == nil {
		t.Fatal("expected error for 400 status, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "status 400") {
		t.Errorf("error = %q, want it to mention status 400", msg)
	}
	if !strings.Contains(msg, "user key is invalid") {
		t.Errorf("error = %q, want it to surface Pushover's errors field", msg)
	}
}

func TestPushoverNotifier_ErrorsNeverContainSecrets(t *testing.T) {
	const token = "app-token-secret"
	const user = "user-key-secret"

	t.Run("error body echoes secrets", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			// A misbehaving server that echoes credentials must not leak them
			// into the error we return.
			_, _ = w.Write([]byte(`{"status":0,"errors":["bad token ` + token + ` for user ` + user + `"]}`))
		}))
		defer srv.Close()

		n := NewPushoverNotifier(&config.Config{PushoverToken: token, PushoverUser: user})
		n.endpoint = srv.URL

		err := n.Send(context.Background(), "t", "m")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		msg := err.Error()
		if strings.Contains(msg, token) {
			t.Errorf("error leaks application token: %q", msg)
		}
		if strings.Contains(msg, user) {
			t.Errorf("error leaks user key: %q", msg)
		}
		if !strings.Contains(msg, "[redacted]") {
			t.Errorf("error = %q, want redaction marker", msg)
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		n := NewPushoverNotifier(&config.Config{PushoverToken: token, PushoverUser: user})
		n.endpoint = "http://127.0.0.1:1"

		err := n.Send(context.Background(), "t", "m")
		if err == nil {
			t.Fatal("expected error for unreachable server, got nil")
		}
		msg := err.Error()
		if strings.Contains(msg, token) || strings.Contains(msg, user) {
			t.Errorf("error leaks secrets: %q", msg)
		}
	})
}
