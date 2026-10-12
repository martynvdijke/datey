package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Sender sends a message to a Telegram chat.
type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

// httpsSender posts to the Telegram Bot API.
type httpsSender struct {
	token  string
	client *http.Client
}

// NewHTTPSender creates a Sender that posts to https://api.telegram.org/bot<token>/sendMessage.
func NewHTTPSender(token string) Sender {
	return &httpsSender{
		token:  token,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *httpsSender) SendMessage(ctx context.Context, chatID int64, text string) error {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.token)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram returned status %d", resp.StatusCode)
	}
	return nil
}
