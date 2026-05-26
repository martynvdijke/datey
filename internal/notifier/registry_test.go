package notifier

import (
	"context"
	"testing"
)

type mockNotifier struct {
	name      string
	configured bool
	sent      []string
}

func (m *mockNotifier) Send(_ context.Context, title, message string) error {
	m.sent = append(m.sent, title+": "+message)
	return nil
}

func (m *mockNotifier) Name() string { return m.name }

func (m *mockNotifier) IsConfigured() bool { return m.configured }

func TestRegistry_SendAll_SendsOnlyConfigured(t *testing.T) {
	reg := NewRegistry()
	active := &mockNotifier{name: "active", configured: true}
	inactive := &mockNotifier{name: "inactive", configured: false}

	reg.Register(active)
	reg.Register(inactive)

	reg.SendAll(context.Background(), "Test", "Hello")

	if len(active.sent) != 1 {
		t.Errorf("active notifier should have 1 message, got %d", len(active.sent))
	}
	if len(inactive.sent) != 0 {
		t.Errorf("inactive notifier should have 0 messages, got %d", len(inactive.sent))
	}
}

func TestRegistry_IsConfigured(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockNotifier{name: "email", configured: true})
	reg.Register(&mockNotifier{name: "gotify", configured: false})

	if !reg.IsConfigured("email") {
		t.Error("email should be configured")
	}
	if reg.IsConfigured("gotify") {
		t.Error("gotify should not be configured")
	}
	if reg.IsConfigured("telegram") {
		t.Error("telegram should not be configured (not registered)")
	}
}
