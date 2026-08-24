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
	return n.SendTo(ctx, title, message, "")
}

func (n *DiscordNotifier) SendTo(ctx context.Context, title, message string, target string) error {
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
