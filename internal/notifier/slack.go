package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/datey/datey/internal/config"
)

type SlackNotifier struct {
	cfg    *config.Config
	client *http.Client
}

func NewSlackNotifier(cfg *config.Config) *SlackNotifier {
	return &SlackNotifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *SlackNotifier) Name() string { return "slack" }

func (n *SlackNotifier) IsConfigured() bool {
	return n.cfg.SlackWebhookURL != ""
}

func (n *SlackNotifier) Send(ctx context.Context, title, message string) error {
	return n.SendTo(ctx, title, message, "")
}

func (n *SlackNotifier) SendTo(ctx context.Context, title, message string, target string) error {
	dest := n.cfg.SlackWebhookURL
	if target != "" {
		dest = target
	}
	text := fmt.Sprintf("%s\n%s", title, message)
	payload := map[string]any{
		"text": text,
		"blocks": []map[string]any{
			{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*%s*\n%s", title, message),
				},
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
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(respBody))
	switch {
	case trimmed == "ok", trimmed == "":
		return nil
	case strings.Contains(trimmed, "msg_too_long"):
		return fmt.Errorf("slack error: msg_too_long")
	default:
		// Slack incoming webhooks usually return "ok" on success; anything else is treated as error if it looks like error.
		// But spec requires surfacing msg_too_long; other non-ok bodies are also failures.
		if trimmed != "ok" {
			// If body is JSON with error field, handle generically.
			return fmt.Errorf("slack error: %s", trimmed)
		}
		return nil
	}
}
