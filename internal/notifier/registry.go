package notifier

import (
	"context"
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
	slog.Info("registered notifier", "name", n.Name(), "configured", n.IsConfigured())
}

func (r *Registry) SendAll(ctx context.Context, title, message string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, n := range r.notifiers {
		if !n.IsConfigured() {
			continue
		}
		if err := n.Send(ctx, title, message); err != nil {
			slog.Error("notification failed", "channel", name, "error", err)
		} else {
			slog.Info("notification sent", "channel", name)
		}
	}
}

func (r *Registry) IsConfigured(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.notifiers[name]
	return ok && n.IsConfigured()
}
