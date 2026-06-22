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

type GotifyNotifier struct {
	cfg    *config.Config
	client *http.Client
}

func NewGotifyNotifier(cfg *config.Config) *GotifyNotifier {
	return &GotifyNotifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *GotifyNotifier) Name() string { return "gotify" }

func (n *GotifyNotifier) IsConfigured() bool {
	return n.cfg.GotifyURL != "" && n.cfg.GotifyToken != ""
}

func (n *GotifyNotifier) Send(ctx context.Context, title, message string) error {
	payload := map[string]any{
		"title":   title,
		"message": message,
		"priority": 5,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", n.cfg.GotifyURL+"/message", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("X-Gotify-Key", n.cfg.GotifyToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gotify returned status %d", resp.StatusCode)
	}

	return nil
}
