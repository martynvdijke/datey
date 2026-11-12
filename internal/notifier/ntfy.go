package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/datey/datey/internal/config"
)

// NtfyNotifier publishes messages to a ntfy.sh-style server via the ntfy
// publish API: a JSON POST to /{topic} with topic, title, message and an
// optional priority (1-5, higher = more urgent).
type NtfyNotifier struct {
	cfg    *config.Config
	client *http.Client
}

func NewNtfyNotifier(cfg *config.Config) *NtfyNotifier {
	return &NtfyNotifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *NtfyNotifier) Name() string { return "ntfy" }

func (n *NtfyNotifier) IsConfigured() bool {
	return n.cfg.NtfyTopic != ""
}

func (n *NtfyNotifier) Send(ctx context.Context, title, message string) error {
	return n.SendToActions(ctx, title, message, "", ActionLinks{})
}

func (n *NtfyNotifier) SendTo(ctx context.Context, title, message string, target string) error {
	return n.SendToActions(ctx, title, message, target, ActionLinks{})
}

// SendToActions publishes the reminder with optional native action buttons.
// ntfy renders them under the message; each is a "view" action that opens the
// signed confirmation page in the browser, which keeps the action working
// without an ntfy account and without a CSRF-protected direct POST.
func (n *NtfyNotifier) SendToActions(ctx context.Context, title, message, target string, actions ActionLinks) error {
	baseURL := n.cfg.NtfyURL
	if baseURL == "" {
		baseURL = "https://ntfy.sh"
	}
	topic := n.cfg.NtfyTopic
	if target != "" {
		topic = target
	}
	priority := n.cfg.NtfyPriority
	if priority < 1 || priority > 5 {
		priority = 3
	}
	payload := map[string]any{
		"topic":    topic,
		"title":    title,
		"message":  message,
		"priority": priority,
	}
	if buttons := ntfyActions(actions); len(buttons) > 0 {
		payload["actions"] = buttons
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/"+topic, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if n.cfg.NtfyToken != "" {
		req.Header.Set("Authorization", "Bearer "+n.cfg.NtfyToken)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ntfy returned status %d", resp.StatusCode)
	}

	return nil
}

// ntfyActions renders the two reminder actions as ntfy "view" actions. Labels
// are fixed English: notifiers have no per-recipient locale, and the
// confirmation page they open is localised.
func ntfyActions(actions ActionLinks) []map[string]any {
	out := make([]map[string]any, 0, 2)
	if actions.SnoozeURL != "" {
		out = append(out, map[string]any{
			"action": "view",
			"label":  "Snooze 3 days",
			"url":    actions.SnoozeURL,
			"clear":  true,
		})
	}
	if actions.AckURL != "" {
		out = append(out, map[string]any{
			"action": "view",
			"label":  "Acknowledge",
			"url":    actions.AckURL,
			"clear":  true,
		})
	}
	return out
}
