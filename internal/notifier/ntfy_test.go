package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/datey/datey/internal/config"
)

func TestNtfyNotifier_Name(t *testing.T) {
	n := NewNtfyNotifier(&config.Config{})
	if got := n.Name(); got != "ntfy" {
		t.Errorf("Name() = %q, want %q", got, "ntfy")
	}
}

func TestNtfyNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"topic set", &config.Config{NtfyTopic: "reminders"}, true},
		{"topic empty", &config.Config{NtfyTopic: ""}, false},
		{"topic whitespace", &config.Config{NtfyTopic: "  "}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNtfyNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNtfyNotifier_Send_PayloadShape(t *testing.T) {
	var mu sync.Mutex
	var gotPath, gotAuth, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNtfyNotifier(&config.Config{
		NtfyURL:      srv.URL,
		NtfyTopic:    "reminders",
		NtfyToken:    "tk-123",
		NtfyPriority: 5,
	})

	if err := n.Send(context.Background(), "Reminder: Dana - birthday", "Dana's birthday is in 3 days"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if gotPath != "/reminders" {
		t.Errorf("request path = %q, want %q", gotPath, "/reminders")
	}
	if gotAuth != "Bearer tk-123" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer tk-123")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("response body not valid JSON: %v", err)
	}
	if payload["topic"] != "reminders" {
		t.Errorf("payload topic = %v, want %q", payload["topic"], "reminders")
	}
	if payload["title"] != "Reminder: Dana - birthday" {
		t.Errorf("payload title = %v", payload["title"])
	}
	if payload["message"] != "Dana's birthday is in 3 days" {
		t.Errorf("payload message = %v", payload["message"])
	}
	if payload["priority"] != float64(5) {
		t.Errorf("payload priority = %v, want 5", payload["priority"])
	}
}

func TestNtfyNotifier_Send_NoAuthHeaderWithoutToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNtfyNotifier(&config.Config{NtfyURL: srv.URL, NtfyTopic: "pub"})
	if err := n.Send(context.Background(), "t", "m"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization header set without token: %q", gotAuth)
	}
}

func TestNtfyNotifier_Send_ClampsOutOfRangePriority(t *testing.T) {
	// Send must clamp an out-of-range configured priority to 3.
	var gotPriority float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotPriority = payload["priority"].(float64)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNtfyNotifier(&config.Config{NtfyURL: srv.URL, NtfyTopic: "reminders", NtfyPriority: 99})
	if err := n.Send(context.Background(), "t", "m"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotPriority != 3 {
		t.Errorf("clamped priority = %v, want 3", gotPriority)
	}
}

func TestNtfyNotifier_Send_ErrorPropagation(t *testing.T) {
	t.Run("server error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		n := NewNtfyNotifier(&config.Config{NtfyURL: srv.URL, NtfyTopic: "x"})
		err := n.Send(context.Background(), "t", "m")
		if err == nil {
			t.Fatal("expected error for 500 status, got nil")
		}
		if !strings.Contains(err.Error(), "status 500") {
			t.Errorf("error = %q, want it to mention status 500", err)
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		n := NewNtfyNotifier(&config.Config{NtfyURL: "http://127.0.0.1:1", NtfyTopic: "x"})
		if err := n.Send(context.Background(), "t", "m"); err == nil {
			t.Error("expected error for unreachable server, got nil")
		}
	})
}
