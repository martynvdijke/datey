package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/datey/datey/internal/i18n"
)

// Notification is one in-app realtime message delivered over SSE.
type Notification struct {
	Title   string    `json:"title"`
	Message string    `json:"message"`
	URL     string    `json:"url"`
	Kind    string    `json:"kind"`
	At      time.Time `json:"at"`
}

// brokerBuffer is how many pending notifications a single subscriber may have
// before messages are dropped for that subscriber only.
const brokerBuffer = 16

// ssePingInterval keeps proxies and browsers from closing an idle SSE stream.
const ssePingInterval = 25 * time.Second

// Broker fans notifications out to per-user SSE subscribers. Publishing never
// blocks: a subscriber whose buffer is full drops the message.
type Broker struct {
	mu   sync.RWMutex
	subs map[int][]chan Notification
}

// NewBroker returns an empty broker, ready for use.
func NewBroker() *Broker {
	return &Broker{subs: make(map[int][]chan Notification)}
}

// Subscribe registers a subscriber for userID. The returned cancel function
// removes the subscription and closes the channel; it is safe to call twice.
func (b *Broker) Subscribe(userID int) (<-chan Notification, func()) {
	ch := make(chan Notification, brokerBuffer)

	b.mu.Lock()
	b.subs[userID] = append(b.subs[userID], ch)
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			list := b.subs[userID]
			for i, c := range list {
				if c == ch {
					b.subs[userID] = append(list[:i], list[i+1:]...)
					break
				}
			}
			if len(b.subs[userID]) == 0 {
				delete(b.subs, userID)
			}
			// Publish holds the read lock while sending, so closing here can
			// never race with a send on this channel.
			close(ch)
		})
	}
	return ch, cancel
}

// Publish delivers n to every subscriber of the given users, dropping the
// message for any subscriber whose buffer is full.
func (b *Broker) Publish(userIDs []int, n Notification) {
	if b == nil || len(userIDs) == 0 {
		return
	}
	if n.At.IsZero() {
		n.At = time.Now()
	}

	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, userID := range userIDs {
		for _, ch := range b.subs[userID] {
			select {
			case ch <- n:
			default:
				// Slow or stuck subscriber: keep the publisher non-blocking.
			}
		}
	}
}

// PublishNewItem builds a Notification from a new-item event and publishes it.
// It is the concrete implementation of the dispatcher's publisher interface.
func (b *Broker) PublishNewItem(userIDs []int, kind, title, message, url string) {
	b.Publish(userIDs, Notification{
		Title:   title,
		Message: message,
		URL:     url,
		Kind:    kind,
		At:      time.Now(),
	})
}

// notifyNewItem dispatches a new-item notification for the current request.
// Actor resolution is nil-safe, and the call never blocks or fails the request:
// the dispatcher does its work in a detached goroutine.
func (h *Handler) notifyNewItem(r *http.Request, kind, title, message, url string) {
	if h == nil || h.newItems == nil {
		return
	}
	actorID := 0
	if u := UserFromContext(r.Context()); u != nil {
		actorID = u.ID
	}
	h.newItems.NotifyNewItem(r.Context(), actorID, kind, title, message, url)
}

// trOr returns the translation for key, falling back to fallbackFormat when
// the key is missing from every catalog (e.g. a locale not yet updated).
func trOr(locale, key, fallbackFormat string) string {
	v := i18n.T(locale, key)
	if v == key {
		return fallbackFormat
	}
	return v
}

// formatNewItemMessage renders a localised message template with args. A
// template without a %s verb is returned unchanged.
func formatNewItemMessage(locale, key, fallbackFormat string, args ...any) string {
	format := trOr(locale, key, fallbackFormat)
	if !strings.Contains(format, "%s") {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// eventsStream serves the authenticated SSE feed for the current user.
func (h *Handler) eventsStream(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	broker := h.broker
	if broker == nil {
		broker = NewBroker()
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// SSE streams outlive the server's global WriteTimeout; clear the write
	// deadline for this connection only.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch, cancel := broker.Subscribe(u.ID)
	defer cancel()

	_, _ = fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()

	ping := time.NewTicker(ssePingInterval)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case n, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(n)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: notification\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
