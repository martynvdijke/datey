package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/datey/datey/internal/config"
)

const (
	// defaultPushoverEndpoint is Pushover's message API endpoint.
	defaultPushoverEndpoint = "https://api.pushover.net/1/messages.json"
	// pushoverErrorBodyLimit caps how much of an error response is surfaced, so
	// a hostile or verbose server cannot flood logs.
	pushoverErrorBodyLimit = 512
)

// PushoverNotifier delivers reminders through Pushover. The application token
// identifies the sender; the user key selects the recipient and can be
// overridden per user through SendTo.
type PushoverNotifier struct {
	cfg      *config.Config
	client   *http.Client
	endpoint string
}

func NewPushoverNotifier(cfg *config.Config) *PushoverNotifier {
	return &PushoverNotifier{
		cfg:      cfg,
		client:   &http.Client{Timeout: 10 * time.Second},
		endpoint: defaultPushoverEndpoint,
	}
}

func (n *PushoverNotifier) Name() string { return "pushover" }

// IsConfigured reports whether the application token is present. The recipient
// user key may come from the shared default or from a per-user target, so a
// missing default key does not disable the channel by itself.
func (n *PushoverNotifier) IsConfigured() bool {
	return strings.TrimSpace(n.cfg.PushoverToken) != ""
}

func (n *PushoverNotifier) Send(ctx context.Context, title, message string) error {
	return n.SendTo(ctx, title, message, "")
}

func (n *PushoverNotifier) SendTo(ctx context.Context, title, message string, target string) error {
	token := strings.TrimSpace(n.cfg.PushoverToken)
	if token == "" {
		return fmt.Errorf("pushover: application token not configured")
	}

	user := strings.TrimSpace(n.cfg.PushoverUser)
	if t := strings.TrimSpace(target); t != "" {
		user = t
	}
	if user == "" {
		return fmt.Errorf("pushover: recipient user key not configured")
	}

	endpoint := n.endpoint
	if endpoint == "" {
		endpoint = defaultPushoverEndpoint
	}

	form := url.Values{
		"token":    {token},
		"user":     {user},
		"message":  {message},
		"title":    {title},
		"priority": {"0"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pushover returned status %d: %s", resp.StatusCode, pushoverErrorDetail(resp.Body, token, user))
	}

	return nil
}

// pushoverErrorDetail renders Pushover's error response, preferring the
// human-readable "errors" list it returns. Secrets are redacted and the body is
// truncated so the resulting error stays safe to log.
func pushoverErrorDetail(body io.Reader, secrets ...string) string {
	raw, _ := io.ReadAll(io.LimitReader(body, pushoverErrorBodyLimit))
	detail := strings.TrimSpace(string(raw))

	var payload struct {
		Status int      `json:"status"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil && len(payload.Errors) > 0 {
		detail = strings.Join(payload.Errors, "; ")
	}

	for _, secret := range secrets {
		if secret != "" {
			detail = strings.ReplaceAll(detail, secret, "[redacted]")
		}
	}

	if detail == "" {
		return "empty response body"
	}
	return detail
}
