package web

import (
	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/search"
	"github.com/datey/datey/internal/telegram"
)

// telegramSearchLimit caps how many index rows the bot reads per /search. The
// command renders far fewer, so this only bounds worst-case work.
const telegramSearchLimit = 25

// EnableTelegramSearch attaches the FTS5 full-text index to the Telegram bot so
// /search can answer queries. It is a no-op when the Telegram handler is not
// the default *telegram.Bot, or when the ent client has no registered *sql.DB
// (as in tests that build a bare client) — the bot then replies "search
// unavailable" instead of panicking.
func (h *Handler) EnableTelegramSearch() {
	bot, ok := h.telegram.(*telegram.Bot)
	if !ok {
		return
	}
	conn, ok := db.SQLDB(h.client)
	if !ok {
		return
	}
	searcher := search.New(conn)
	searcher.Limit = telegramSearchLimit
	bot.SetSearcher(searcher)
}
