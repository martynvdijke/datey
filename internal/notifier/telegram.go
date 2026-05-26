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

type TelegramNotifier struct {
	cfg    *config.Config
	client *http.Client
}

func NewTelegramNotifier(cfg *config.Config) *TelegramNotifier {
	return &TelegramNotifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *TelegramNotifier) Name() string { return "telegram" }

func (n *TelegramNotifier) IsConfigured() bool {
	return n.cfg.TelegramBotToken != "" && n.cfg.TelegramChatID != ""
}

func (n *TelegramNotifier) Send(ctx context.Context, title, message string) error {
	text := fmt.Sprintf("*%s*\n%s", title, message)

	payload := map[string]any{
		"chat_id":    n.cfg.TelegramChatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.cfg.TelegramBotToken)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram returned status %d", resp.StatusCode)
	}

	return nil
}
