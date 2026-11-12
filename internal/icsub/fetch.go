// Package icsub fetches remote iCalendar feeds and keeps subscription-owned
// events in sync with them by UID.
package icsub

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/datey/datey/internal/ics"
)

const (
	// DefaultTimeout bounds a single feed download.
	DefaultTimeout = 30 * time.Second
	// MaxFeedSize bounds the number of bytes read from a feed.
	MaxFeedSize = 10 << 20 // 10 MB
)

// Fetcher retrieves and parses one ICS feed.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) ([]ics.Event, error)
}

// HTTPFetcher downloads feeds over HTTP(S). It is deliberately lenient about
// the Content-Type header (many calendar servers send text/plain or
// application/octet-stream) and instead sniffs the payload for a VCALENDAR
// header.
type HTTPFetcher struct {
	Client   *http.Client
	MaxBytes int64
}

// NewHTTPFetcher returns a fetcher with a 30 second timeout and a 10 MB cap.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{
		Client:   &http.Client{Timeout: DefaultTimeout},
		MaxBytes: MaxFeedSize,
	}
}

// Fetch downloads rawURL and parses the response as iCalendar.
func (f *HTTPFetcher) Fetch(ctx context.Context, rawURL string) ([]ics.Event, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid ICS feed URL %q", rawURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "text/calendar, text/plain;q=0.5, */*;q=0.1")
	req.Header.Set("User-Agent", "Datey-ICS-Subscriber/1.0")

	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("fetch feed: unexpected HTTP status %s", resp.Status)
	}

	limit := f.MaxBytes
	if limit <= 0 {
		limit = MaxFeedSize
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read feed: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("feed exceeds maximum size of %d bytes", limit)
	}
	if !looksLikeCalendar(body) {
		return nil, fmt.Errorf("response is not an iCalendar feed (Content-Type %q)", resp.Header.Get("Content-Type"))
	}

	events, err := ics.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	return events, nil
}

// looksLikeCalendar sniffs a payload for the VCALENDAR prologue, tolerating a
// UTF-8 BOM and leading blank lines.
func looksLikeCalendar(data []byte) bool {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	data = bytes.TrimLeft(data, " \t\r\n")
	if len(data) > 64 {
		data = data[:64]
	}
	return bytes.HasPrefix(bytes.ToUpper(data), []byte("BEGIN:VCALENDAR"))
}
