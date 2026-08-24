package notifier

import (
	"bytes"
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

type MatrixNotifier struct {
	cfg    *config.Config
	client *http.Client
}

func NewMatrixNotifier(cfg *config.Config) *MatrixNotifier {
	return &MatrixNotifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *MatrixNotifier) Name() string { return "matrix" }

func (n *MatrixNotifier) IsConfigured() bool {
	return n.cfg.MatrixHomeserverURL != "" && n.cfg.MatrixAccessToken != "" && n.cfg.MatrixRoomID != ""
}

func (n *MatrixNotifier) Send(ctx context.Context, title, message string) error {
	return n.SendTo(ctx, title, message, "")
}

func (n *MatrixNotifier) SendTo(ctx context.Context, title, message string, target string) error {
	homeserver := strings.TrimRight(n.cfg.MatrixHomeserverURL, "/")
	roomID := n.cfg.MatrixRoomID
	if target != "" {
		roomID = target
	}
	roomID = url.PathEscape(roomID)
	txnID := fmt.Sprintf("%d", time.Now().UnixNano())

	endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s", homeserver, roomID, txnID)

	plain := fmt.Sprintf("%s\n%s", title, message)
	formatted := fmt.Sprintf("<b>%s</b><br>%s", htmlEscape(title), htmlEscape(message))

	payload := map[string]any{
		"msgtype":        "m.text",
		"body":           plain,
		"format":         "org.matrix.custom.html",
		"formatted_body": formatted,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+n.cfg.MatrixAccessToken)

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	var errResp struct {
		Errcode string `json:"errcode"`
		Error   string `json:"error"`
	}
	if jsonErr := json.Unmarshal(respBody, &errResp); jsonErr == nil && errResp.Errcode != "" {
		switch errResp.Errcode {
		case "M_FORBIDDEN":
			return fmt.Errorf("matrix error M_FORBIDDEN: %s", errResp.Error)
		case "M_UNKNOWN_TOKEN":
			return fmt.Errorf("matrix error M_UNKNOWN_TOKEN: %s", errResp.Error)
		default:
			return fmt.Errorf("matrix error %s: %s", errResp.Errcode, errResp.Error)
		}
	}

	return fmt.Errorf("matrix returned status %d", resp.StatusCode)
}

func htmlEscape(s string) string {
	r := strings.ReplaceAll(s, "&", "&amp;")
	r = strings.ReplaceAll(r, "<", "&lt;")
	r = strings.ReplaceAll(r, ">", "&gt;")
	r = strings.ReplaceAll(r, `"`, "&quot;")
	return r
}
