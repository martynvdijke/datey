package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datey/datey/internal/config"
)

var testActions = ActionLinks{
	SnoozeURL: "https://datey.example/a/snooze-token",
	AckURL:    "https://datey.example/a/ack-token",
}

func TestActionLinks_Empty(t *testing.T) {
	if !(ActionLinks{}).Empty() {
		t.Error("zero ActionLinks should be empty")
	}
	if (ActionLinks{SnoozeURL: "x"}).Empty() {
		t.Error("ActionLinks with a snooze URL should not be empty")
	}
}

func TestNtfy_SendToActions_PayloadShape(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNtfyNotifier(&config.Config{NtfyURL: srv.URL, NtfyTopic: "reminders"})
	if err := n.SendToActions(context.Background(), "t", "m", "", testActions); err != nil {
		t.Fatalf("SendToActions() error = %v", err)
	}

	actions, ok := got["actions"].([]any)
	if !ok || len(actions) != 2 {
		t.Fatalf("actions = %v, want 2 entries", got["actions"])
	}
	first := actions[0].(map[string]any)
	if first["action"] != "view" || first["url"] != testActions.SnoozeURL {
		t.Errorf("first action = %v, want view %q", first, testActions.SnoozeURL)
	}
	second := actions[1].(map[string]any)
	if second["label"] != "Acknowledge" || second["url"] != testActions.AckURL {
		t.Errorf("second action = %v", second)
	}
}

func TestNtfy_SendToActions_OmitsButtonsWithoutURLs(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNtfyNotifier(&config.Config{NtfyURL: srv.URL, NtfyTopic: "t"})
	if err := n.Send(context.Background(), "t", "m"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if _, ok := got["actions"]; ok {
		t.Errorf("payload has actions without URLs: %v", got["actions"])
	}
}

func TestDiscord_SendToActions_Components(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	n := NewDiscordNotifier(&config.Config{DiscordWebhookURL: srv.URL})
	if err := n.SendToActions(context.Background(), "t", "m", "", testActions); err != nil {
		t.Fatalf("SendToActions() error = %v", err)
	}

	rows, ok := got["components"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("components = %v, want one row", got["components"])
	}
	row := rows[0].(map[string]any)
	if row["type"] != float64(1) {
		t.Errorf("component row type = %v, want 1", row["type"])
	}
	buttons := row["components"].([]any)
	if len(buttons) != 2 {
		t.Fatalf("buttons = %v, want 2", buttons)
	}
	btn := buttons[1].(map[string]any)
	if btn["style"] != float64(5) || btn["url"] != testActions.AckURL {
		t.Errorf("ack button = %v, want link button to %q", btn, testActions.AckURL)
	}
}

func TestDiscord_Send_OmitComponents(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	n := NewDiscordNotifier(&config.Config{DiscordWebhookURL: srv.URL})
	if err := n.Send(context.Background(), "t", "m"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if _, ok := got["components"]; ok {
		t.Errorf("payload has components without URLs: %v", got["components"])
	}
}

// actionRecorder implements the optional ActionNotifier interface.
type actionRecorder struct {
	mockNotifier
	got ActionLinks
}

func (a *actionRecorder) SendToActions(_ context.Context, title, message, _ string, actions ActionLinks) error {
	a.got = actions
	a.sent = append(a.sent, title+": "+message)
	return nil
}

func TestRegistry_SendToWithActions_PrefersActionNotifier(t *testing.T) {
	reg := NewRegistry()
	rec := &actionRecorder{mockNotifier: mockNotifier{name: "ntfy", configured: true}}
	plain := &mockNotifier{name: "email", configured: true}
	reg.Register(rec)
	reg.Register(plain)

	if err := reg.SendToWithActions(context.Background(), "ntfy", "t", "m", "", testActions); err != nil {
		t.Fatalf("SendToWithActions() error = %v", err)
	}
	if rec.got != testActions {
		t.Errorf("actions delivered = %v, want %v", rec.got, testActions)
	}
	if len(rec.sent) != 1 {
		t.Errorf("action notifier sends = %d, want 1", len(rec.sent))
	}

	// A channel without native actions still gets the plain message.
	if err := reg.SendToWithActions(context.Background(), "email", "t", "m", "", testActions); err != nil {
		t.Fatalf("SendToWithActions() error = %v", err)
	}
	if len(plain.sent) != 1 {
		t.Errorf("plain notifier sends = %d, want 1", len(plain.sent))
	}
}
