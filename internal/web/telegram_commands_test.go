package web

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/telegram"
)

// captureSender records outgoing Telegram messages so the wiring can be
// exercised without any network call.
type captureSender struct{ msgs []string }

func (c *captureSender) SendMessage(_ context.Context, _ int64, text string) error {
	c.msgs = append(c.msgs, text)
	return nil
}

func (c *captureSender) last() string {
	if len(c.msgs) == 0 {
		return ""
	}
	return c.msgs[len(c.msgs)-1]
}

func TestEnableTelegramSearch_WiresIndex(t *testing.T) {
	h, client := newSearchTestHandler(t, "tg_search")
	ctx := context.Background()
	if err := db.EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure search index: %v", err)
	}
	now := time.Now()
	if _, err := client.Person.Create().SetName("Alice Smith").
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create person: %v", err)
	}

	sender := &captureSender{}
	bot := telegram.NewWithSender(h.cfg, h.events, h.people, sender)
	h.SetTelegramHandler(bot)
	h.EnableTelegramSearch()

	h.telegram.HandleUpdate(ctx, &telegram.Update{
		Message: &telegram.Message{Chat: &telegram.Chat{ID: 1}, Text: "/search alice"},
	})
	if !strings.Contains(sender.last(), "Alice Smith") {
		t.Fatalf("expected search hit through wired index, got %q", sender.last())
	}
}

func TestEnableTelegramSearch_NoOps(t *testing.T) {
	// A non-Bot handler must not panic and must not be replaced.
	h, _ := newSearchTestHandler(t, "tg_noop")
	f := &fakeTGHandler{}
	h.SetTelegramHandler(f)
	h.EnableTelegramSearch()
	if h.telegram != f {
		t.Fatalf("expected handler to be left untouched")
	}

	// A handler whose client has no registered SQL handle is a no-op too.
	h2 := newTestWebHandler(t)
	bot := telegram.NewWithSender(h2.cfg, h2.events, h2.people, &captureSender{})
	h2.SetTelegramHandler(bot)
	h2.EnableTelegramSearch() // must not panic

	bot.HandleUpdate(context.Background(), &telegram.Update{
		Message: &telegram.Message{Chat: &telegram.Chat{ID: 1}, Text: "/search alice"},
	})
}

type fakeTGHandler struct{}

func (f *fakeTGHandler) HandleUpdate(context.Context, *telegram.Update) {}
