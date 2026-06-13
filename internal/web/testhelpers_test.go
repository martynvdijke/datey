package web

import (
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	_ "github.com/mattn/go-sqlite3"
)

func newTestWebHandler(t *testing.T) *Handler {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_web_handler?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	cfg := &config.Config{ReminderDays: 7}
	reg := notifier.NewRegistry()
	store := logstore.NewStore(100)
	return NewHandler(cfg, client, reg, store)
}
