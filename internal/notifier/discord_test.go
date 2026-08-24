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

func TestDiscordNotifier_Name(t *testing.T) {
	n := NewDiscordNotifier(&config.Config{})
	if got := n.Name(); got != "discord" {
		t.Errorf("Name() = %q, want discord", got)
	}
}

func TestDiscordNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"set", &config.Config{DiscordWebhookURL: "https://discord.com/api/webhooks/123/abc"}, true},
		{"empty", &config.Config{DiscordWebhookURL: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewDiscordNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiscordNotifier_Send_PayloadShape(t *testing.T) {
	var mu sync.Mutex
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewDiscordNotifier(&config.Config{DiscordWebhookURL: srv.URL})
	if err := n.Send(context.Background(), "Reminder: Dana - birthday", "Dana's birthday is in 3 days"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	var payload map[string]any
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if payload["content"] != "Reminder: Dana - birthday\nDana's birthday is in 3 days" {
		t.Errorf("content = %v", payload["content"])
	}
	embeds, ok := payload["embeds"].([]any)
	if !ok || len(embeds) == 0 {
		t.Fatalf("embeds missing")
	}
	embed, ok := embeds[0].(map[string]any)
	if !ok {
		t.Fatalf("embed not object")
	}
	if embed["title"] != "Reminder: Dana - birthday" {
		t.Errorf("embed title = %v", embed["title"])
	}
	if embed["description"] != "Dana's birthday is in 3 days" {
		t.Errorf("embed description = %v", embed["description"])
	}
	if embed["color"] == nil {
		t.Errorf("embed color missing")
	}
}

func TestDiscordNotifier_Send_ErrorPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	n := NewDiscordNotifier(&config.Config{DiscordWebhookURL: srv.URL})
	err := n.Send(context.Background(), "t", "m")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("error = %q, want status 500", err)
	}
}
