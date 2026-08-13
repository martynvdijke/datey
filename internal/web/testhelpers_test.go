package web

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	_ "github.com/mattn/go-sqlite3"
)

func newTestWebHandler(t *testing.T) *Handler {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_web_handler?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	cfg := &config.Config{ReminderDays: 7, SchedulerCatchup: true}
	reg := notifier.NewRegistry()
	store := logstore.NewStore(100)
	return NewHandler(cfg, client, reg, store)
}

func withUserContext(ctx context.Context) context.Context {
	u := &ent.User{
		ID:       1,
		Username: "testuser",
		Role:     user.RoleAdmin,
	}
	return context.WithValue(ctx, userContextKey, u)
}

// mockNotifier is a test double for notifier.Notifier.
type mockNotifier struct {
	name       string
	configured bool
	sent       []mockMessage
	sendErr    error
}

type mockMessage struct {
	title   string
	message string
}

func (m *mockNotifier) Send(ctx context.Context, title, message string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, mockMessage{title: title, message: message})
	return nil
}

func (m *mockNotifier) Name() string       { return m.name }
func (m *mockNotifier) IsConfigured() bool { return m.configured }

// newTestNotificationsHandlerWithMock creates a handler with a mock notifier
// registered under the given channel name.
func newTestNotificationsHandlerWithMock(t *testing.T, mock *mockNotifier) *Handler {
	t.Helper()

	client := enttest.Open(t, dialect.SQLite, "file:test_notif_mock?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	cfg := &config.Config{ReminderDays: 7}
	reg := notifier.NewRegistry()
	reg.Register(mock)
	store := logstore.NewStore(100)

	return NewHandler(cfg, client, reg, store)
}

// itoa is a simple int to string conversion to avoid importing strconv in tests
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
