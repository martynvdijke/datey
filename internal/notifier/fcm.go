package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
)

const (
	// fcmScope is the OAuth scope required by the FCM HTTP v1 API.
	fcmScope = "https://www.googleapis.com/auth/firebase.messaging"
	// fcmEndpointPrefix marks an Android device token stored in
	// push_subscription. Browser subscriptions never start with it.
	fcmEndpointPrefix = "fcm:"
	// fcmDefaultBaseURL is the public FCM endpoint; tests override it.
	fcmDefaultBaseURL = "https://fcm.googleapis.com"
	// fcmAndroidChannelID is the notification channel the Flutter app creates
	// for new-item alerts.
	fcmAndroidChannelID = "datey_new_items"
)

// FCMNotifier delivers notifications to the Android app through Firebase Cloud
// Messaging (HTTP v1). Device tokens are stored as push_subscription rows with
// endpoint "fcm:"+token; tokens the service reports as unregistered or invalid
// are pruned automatically.
type FCMNotifier struct {
	cfg  *config.Config
	subs *repository.PushSubscriptionRepository

	// client is injectable for tests; nil means a 10s-timeout client.
	client *http.Client
	// baseURL is injectable for tests; defaults to fcmDefaultBaseURL.
	baseURL string
	// tokenSource can be injected by tests to bypass service-account auth.
	tokenSource oauth2.TokenSource

	mu sync.Mutex
	ts oauth2.TokenSource
}

// NewFCMNotifier creates an FCMNotifier. Credentials are parsed lazily on the
// first send so a bad service-account JSON never blocks startup.
func NewFCMNotifier(cfg *config.Config, subs *repository.PushSubscriptionRepository) *FCMNotifier {
	return &FCMNotifier{
		cfg:     cfg,
		subs:    subs,
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: fcmDefaultBaseURL,
	}
}

// Name implements Notifier.
func (n *FCMNotifier) Name() string { return "fcm" }

// IsConfigured implements Notifier.
func (n *FCMNotifier) IsConfigured() bool {
	return n.cfg != nil &&
		n.cfg.FCMEnabled &&
		n.cfg.FCMProjectID != "" &&
		n.cfg.FCMServiceAccountJSON != ""
}

// fcmPayload mirrors the subset of the HTTP v1 message shape Datey sends.
type fcmPayload struct {
	Message fcmMessage `json:"message"`
}

type fcmMessage struct {
	Token        string            `json:"token"`
	Notification fcmNotification   `json:"notification"`
	Data         map[string]string `json:"data,omitempty"`
	Android      *fcmAndroidConfig `json:"android,omitempty"`
}

type fcmNotification struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

type fcmAndroidConfig struct {
	Priority     string                  `json:"priority,omitempty"`
	Notification *fcmAndroidNotification `json:"notification,omitempty"`
}

type fcmAndroidNotification struct {
	ChannelID string `json:"channel_id,omitempty"`
}

// fcmErrorResponse is the error envelope returned by the FCM HTTP v1 API.
type fcmErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
		Details []struct {
			Type      string `json:"@type"`
			ErrorCode string `json:"errorCode"`
		} `json:"details"`
	} `json:"error"`
}

// prunable reports whether the response means the device token is dead and the
// stored row should be deleted.
func (e *fcmErrorResponse) prunable() bool {
	if e.Error.Status == "NOT_FOUND" {
		return true
	}
	for _, d := range e.Error.Details {
		if d.ErrorCode == "UNREGISTERED" || d.ErrorCode == "INVALID_ARGUMENT" {
			return true
		}
	}
	return false
}

// Send implements Notifier by broadcasting to every stored FCM device token.
// Per-user channel gating is the dispatcher's job; Send exists so the registry
// can treat FCM like the other channels.
func (n *FCMNotifier) Send(ctx context.Context, title, message string) error {
	return n.broadcast(ctx, title, message, "", "")
}

// SendTo implements Notifier. FCM has no per-channel target text.
func (n *FCMNotifier) SendTo(ctx context.Context, title, message string, target string) error {
	_ = target
	return n.broadcast(ctx, title, message, "", "")
}

// SendToUser delivers a notification to every device token owned by userID.
// It returns the first delivery error, if any; individual failures are logged
// and do not abort the remaining tokens.
func (n *FCMNotifier) SendToUser(ctx context.Context, userID int, title, message, url, kind string) error {
	if !n.IsConfigured() {
		return fmt.Errorf("fcm: not configured")
	}
	subs, err := n.subs.ListByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("fcm: list subscriptions: %w", err)
	}
	var tokens []string
	for _, sub := range subs {
		if tok, ok := fcmToken(sub.Endpoint); ok {
			tokens = append(tokens, tok)
		}
	}
	return n.sendToTokens(ctx, tokens, title, message, url, kind)
}

// broadcast sends to every stored FCM token.
func (n *FCMNotifier) broadcast(ctx context.Context, title, message, url, kind string) error {
	if !n.IsConfigured() {
		return fmt.Errorf("fcm: not configured")
	}
	subs, err := n.subs.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("fcm: list subscriptions: %w", err)
	}
	var tokens []string
	for _, sub := range subs {
		if tok, ok := fcmToken(sub.Endpoint); ok {
			tokens = append(tokens, tok)
		}
	}
	return n.sendToTokens(ctx, tokens, title, message, url, kind)
}

// fcmToken extracts the device token from a stored endpoint, reporting whether
// the row is an FCM device row at all.
func fcmToken(endpoint string) (string, bool) {
	if !strings.HasPrefix(endpoint, fcmEndpointPrefix) {
		return "", false
	}
	tok := strings.TrimPrefix(endpoint, fcmEndpointPrefix)
	if tok == "" {
		return "", false
	}
	return tok, true
}

func (n *FCMNotifier) sendToTokens(ctx context.Context, tokens []string, title, message, url, kind string) error {
	ts, err := n.credentials(ctx)
	if err != nil {
		return fmt.Errorf("fcm: credentials: %w", err)
	}

	var firstErr error
	for _, tok := range tokens {
		if err := n.sendOne(ctx, ts, tok, title, message, url, kind); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// credentials returns the cached OAuth token source, building it from the
// service-account JSON on first use.
func (n *FCMNotifier) credentials(ctx context.Context) (oauth2.TokenSource, error) {
	if n.tokenSource != nil {
		return n.tokenSource, nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ts != nil {
		return n.ts, nil
	}
	creds, err := google.CredentialsFromJSON(ctx, []byte(n.cfg.FCMServiceAccountJSON), fcmScope)
	if err != nil {
		return nil, err
	}
	n.ts = creds.TokenSource
	return n.ts, nil
}

func (n *FCMNotifier) sendOne(ctx context.Context, ts oauth2.TokenSource, token, title, message, url, kind string) error {
	tok, err := ts.Token()
	if err != nil {
		return fmt.Errorf("fcm: obtain access token: %w", err)
	}

	data := map[string]string{}
	if url != "" {
		data["url"] = url
	}
	if kind != "" {
		data["kind"] = kind
	}

	body, err := json.Marshal(fcmPayload{Message: fcmMessage{
		Token:        token,
		Notification: fcmNotification{Title: title, Body: message},
		Data:         data,
		Android: &fcmAndroidConfig{
			Priority:     "high",
			Notification: &fcmAndroidNotification{ChannelID: fcmAndroidChannelID},
		},
	}})
	if err != nil {
		return fmt.Errorf("fcm: marshal payload: %w", err)
	}

	endpoint := strings.TrimSuffix(n.baseURL, "/") + "/v1/projects/" + n.cfg.FCMProjectID + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("fcm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)

	client := n.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fcm: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("fcm: read response: %w", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	var apiErr fcmErrorResponse
	if jsonErr := json.Unmarshal(respBody, &apiErr); jsonErr == nil && apiErr.prunable() {
		// The device is gone (uninstalled or token rotated): drop the row so
		// later sends do not keep failing.
		if delErr := n.subs.DeleteByEndpoint(ctx, fcmEndpointPrefix+token); delErr != nil {
			slog.Error("fcm: prune device token", "source", "notifier", "error", delErr)
		}
		return fmt.Errorf("fcm: token unregistered (pruned), status %d", resp.StatusCode)
	}

	msg := strings.TrimSpace(string(respBody))
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return fmt.Errorf("fcm: send failed with status %d: %s", resp.StatusCode, msg)
}
