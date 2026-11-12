package telegram

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/search"
)

// EventLister lists upcoming occurrences.
type EventLister interface {
	ListUpcomingOccurrences(ctx context.Context, from, to time.Time) ([]repository.EventOccurrence, error)
}

// PersonFinder finds people.
type PersonFinder interface {
	FindByName(ctx context.Context, name string) (*ent.Person, error)
	Search(ctx context.Context, q string) ([]*ent.Person, error)
}

// Searcher queries the full-text index. internal/search.Searcher satisfies it.
type Searcher interface {
	Search(ctx context.Context, query string) ([]search.Group, error)
}

// Bot handles Telegram updates.
type Bot struct {
	cfg    *config.Config
	events EventLister
	people PersonFinder
	search Searcher
	sender Sender
}

// New creates a Bot using NewHTTPSender.
func New(cfg *config.Config, events EventLister, people PersonFinder) *Bot {
	return NewWithSender(cfg, events, people, NewHTTPSender(cfg.TelegramBotToken))
}

// NewWithSender creates a Bot with a custom Sender (for testing).
func NewWithSender(cfg *config.Config, events EventLister, people PersonFinder, sender Sender) *Bot {
	return &Bot{cfg: cfg, events: events, people: people, sender: sender}
}

// SetSearcher attaches the full-text search backend. It may be left unset: the
// /search command then answers with a "search unavailable" message instead of
// failing.
func (b *Bot) SetSearcher(s Searcher) { b.search = s }

func todayStartUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// defaultUpcomingDays is the window used by /upcoming when no argument is given.
const defaultUpcomingDays = 30

// maxMessageRunes leaves headroom below Telegram's 4096-character limit for the
// truncation marker.
const maxMessageRunes = 4000

const truncationMarker = "\n… (truncated)"

// truncateReply keeps a reply within Telegram's message limit. Telegram counts
// UTF-16 code units; capping runes is stricter for BMP text and close enough
// elsewhere, and the headroom above absorbs surrogate pairs.
func truncateReply(s string) string {
	if utf8.RuneCountInString(s) <= maxMessageRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxMessageRunes]) + truncationMarker
}

func helpText() string {
	return "Datey bot — upcoming dates\n" +
		"/upcoming [days] — next N days (default 30, max 365)\n" +
		"/today — today's events\n" +
		"/person <name> — a person's upcoming dates\n" +
		"/search <query> — search people, notes, gifts, events and groups\n" +
		"/help — this message"
}

// HandleUpdate processes a single update.
func (b *Bot) HandleUpdate(ctx context.Context, upd *Update) {
	if upd == nil || upd.Message == nil || upd.Message.Chat == nil {
		return
	}
	text := strings.TrimSpace(upd.Message.Text)
	if text == "" {
		return
	}
	chatID := upd.Message.Chat.ID

	// Only handle commands starting with '/'
	if !strings.HasPrefix(text, "/") {
		_ = b.send(ctx, chatID, "Try /help for available commands.")
		return
	}

	// Split command and args.
	// Command may have @botusername suffix: /upcoming@mybot
	spaceIdx := strings.IndexFunc(text, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	var cmdPart, argPart string
	if spaceIdx == -1 {
		cmdPart = text
		argPart = ""
	} else {
		cmdPart = text[:spaceIdx]
		argPart = strings.TrimSpace(text[spaceIdx+1:])
	}
	// Strip @bot suffix (groups send /command@BotName).
	if at := strings.Index(cmdPart, "@"); at != -1 {
		cmdPart = cmdPart[:at]
	}
	cmdPart = strings.ToLower(cmdPart)

	var reply string
	switch cmdPart {
	case "/start", "/help":
		reply = helpText()
	case "/upcoming":
		reply = b.handleUpcoming(ctx, argPart)
	case "/today":
		reply = b.handleToday(ctx)
	case "/person":
		reply = b.handlePerson(ctx, argPart)
	case "/search":
		reply = b.handleSearch(ctx, argPart)
	default:
		reply = "Unknown command. Try /help."
	}

	if err := b.send(ctx, chatID, reply); err != nil {
		slog.Error("telegram send failed", "error", err, "chat_id", chatID, "command", cmdPart)
	}
}

func (b *Bot) send(ctx context.Context, chatID int64, text string) error {
	return b.sender.SendMessage(ctx, chatID, truncateReply(text))
}
