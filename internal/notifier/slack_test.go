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

func TestSlackNotifier_Name(t *testing.T) {
	n := NewSlackNotifier(&config.Config{})
	if got := n.Name(); got != "slack" {
		t.Errorf("Name() = %q, want slack", got)
	}
}

func TestSlackNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"set", &config.Config{SlackWebhookURL: "https://hooks.slack.com/services/xxx"}, true},
		{"empty", &config.Config{SlackWebhookURL: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewSlackNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSlackNotifier_Send_PayloadShape(t *testing.T) {
	var mu sync.Mutex
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	n := NewSlackNotifier(&config.Config{SlackWebhookURL: srv.URL})
	if err := n.Send(context.Background(), "Reminder: Dana - birthday", "Dana's birthday is in 3 days"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	var payload map[string]any
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if payload["text"] != "Reminder: Dana - birthday\nDana's birthday is in 3 days" {
		t.Errorf("text = %v", payload["text"])
	}
	blocks, ok := payload["blocks"].([]any)
	if !ok || len(blocks) == 0 {
		t.Fatalf("blocks missing")
	}
	block, ok := blocks[0].(map[string]any)
	if !ok || block["type"] != "section" {
		t.Fatalf("block = %v", block)
	}
}

func TestSlackNotifier_Send_MsgTooLong(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("msg_too_long"))
	}))
	defer srv.Close()
	n := NewSlackNotifier(&config.Config{SlackWebhookURL: srv.URL})
	err := n.Send(context.Background(), "t", "m")
	if err == nil {
		t.Fatal("expected msg_too_long error")
	}
	if !strings.Contains(err.Error(), "msg_too_long") {
		t.Errorf("error = %q, want msg_too_long", err)
	}
}

func TestSlackNotifier_Send_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	n := NewSlackNotifier(&config.Config{SlackWebhookURL: srv.URL})
	err := n.Send(context.Background(), "t", "m")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("error = %q", err)
	}
}
