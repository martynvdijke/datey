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

type DiscordNotifier struct {
	cfg    *config.Config
	client *http.Client
}

func NewDiscordNotifier(cfg *config.Config) *DiscordNotifier {
	return &DiscordNotifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *DiscordNotifier) Name() string { return "discord" }

func (n *DiscordNotifier) IsConfigured() bool {
	return n.cfg.DiscordWebhookURL != ""
}

func (n *DiscordNotifier) Send(ctx context.Context, title, message string) error {
	return n.SendToActions(ctx, title, message, "", ActionLinks{})
}

func (n *DiscordNotifier) SendTo(ctx context.Context, title, message string, target string) error {
	return n.SendToActions(ctx, title, message, target, ActionLinks{})
}

// SendToActions posts the reminder with optional link buttons. Discord renders
// them as a component row under the embed; each opens the signed confirmation
// page in the browser (style 5 = link button, so no interaction endpoint is
// needed).
func (n *DiscordNotifier) SendToActions(ctx context.Context, title, message, target string, actions ActionLinks) error {
	dest := n.cfg.DiscordWebhookURL
	if target != "" {
		dest = target
	}
	payload := map[string]any{
		"content": fmt.Sprintf("%s\n%s", title, message),
		"embeds": []map[string]any{
			{
				"title":       title,
				"description": message,
				"color":       0x5865F2,
			},
		},
	}
	if components := discordComponents(actions); len(components) > 0 {
		payload["components"] = components
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord returned status %d", resp.StatusCode)
	}
	return nil
}

// discordComponents builds a single action row of link buttons (type 2, style
// 5). Labels are fixed English because a webhook payload has no per-recipient
// locale; the page each button opens is localised.
func discordComponents(actions ActionLinks) []map[string]any {
	buttons := make([]map[string]any, 0, 2)
	if actions.SnoozeURL != "" {
		buttons = append(buttons, map[string]any{
			"type":  2,
			"style": 5,
			"label": "Snooze 3 days",
			"url":   actions.SnoozeURL,
		})
	}
	if actions.AckURL != "" {
		buttons = append(buttons, map[string]any{
			"type":  2,
			"style": 5,
			"label": "Acknowledge",
			"url":   actions.AckURL,
		})
	}
	if len(buttons) == 0 {
		return nil
	}
	return []map[string]any{{"type": 1, "components": buttons}}
}
