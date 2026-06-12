package notifier

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

type Registry struct {
	mu         sync.RWMutex
	notifiers  map[string]Notifier
}

func NewRegistry() *Registry {
	return &Registry{
		notifiers: make(map[string]Notifier),
	}
}

func (r *Registry) Register(n Notifier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notifiers[n.Name()] = n
	slog.Info("registered notifier", "source", "notifier", "name", n.Name(), "configured", n.IsConfigured())
}

func (r *Registry) SendAll(ctx context.Context, title, message string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, n := range r.notifiers {
		if !n.IsConfigured() {
			continue
		}
		if err := n.Send(ctx, title, message); err != nil {
			slog.Error("notification failed", "source", "notifier", "channel", name, "error", err)
		} else {
			slog.Info("notification sent", "source", "notifier", "channel", name)
		}
	}
}

func (r *Registry) Send(ctx context.Context, name, title, message string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.notifiers[name]
	if !ok {
		return fmt.Errorf("notifier %q not registered", name)
	}
	if !n.IsConfigured() {
		return fmt.Errorf("notifier %q not configured", name)
	}
	return n.Send(ctx, title, message)
}

func (r *Registry) IsConfigured(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.notifiers[name]
	return ok && n.IsConfigured()
}

// ConfiguredNames returns the names of all registered notifiers that are
// configured and ready to send.
func (r *Registry) ConfiguredNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name, n := range r.notifiers {
		if n.IsConfigured() {
			names = append(names, name)
		}
	}
	return names
}
