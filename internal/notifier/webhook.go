package notifier

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/datey/datey/internal/config"
)

// WebhookNotifier POSTs reminders as JSON to one or more user-configured URLs.
// The envelope is intentionally minimal and stable: {title, message, channel,
// sent_at}. When WEBHOOK_SECRET is set, each request carries an HMAC-SHA256
// signature header so receivers can verify the request came from datey.
type WebhookNotifier struct {
	cfg    *config.Config
	client *http.Client
}

func NewWebhookNotifier(cfg *config.Config) *WebhookNotifier {
	return &WebhookNotifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *WebhookNotifier) Name() string { return "webhook" }

func (n *WebhookNotifier) IsConfigured() bool {
	return n.cfg.WebhookURL != ""
}

// webhookEnvelope is the stable JSON body delivered to every target URL.
type webhookEnvelope struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Channel string `json:"channel"`
	SentAt  string `json:"sent_at"`
}

// Send delivers the message to every comma-separated WEBHOOK_URL. Each URL
// receives its own POST; a failure on any URL fails the channel's delivery
// status. Sending continues across URLs and the first error is returned.
func (n *WebhookNotifier) Send(ctx context.Context, title, message string) error {
	return n.SendTo(ctx, title, message, "")
}

func (n *WebhookNotifier) SendTo(ctx context.Context, title, message string, target string) error {
	envelope := webhookEnvelope{
		Title:   title,
		Message: message,
		Channel: "webhook",
		SentAt:  time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	webhookURL := n.cfg.WebhookURL
	if target != "" {
		webhookURL = target
	}
	var firstErr error
	for _, rawURL := range strings.Split(webhookURL, ",") {
		u := strings.TrimSpace(rawURL)
		if u == "" {
			continue
		}
		if err := n.post(ctx, u, body); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (n *WebhookNotifier) post(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if n.cfg.WebhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(n.cfg.WebhookSecret))
		_, _ = mac.Write(body)
		req.Header.Set("X-Datey-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request to %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook %s returned status %d", url, resp.StatusCode)
	}

	return nil
}
