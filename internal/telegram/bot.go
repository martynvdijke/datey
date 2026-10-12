package telegram

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
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

// Bot handles Telegram updates.
type Bot struct {
	cfg    *config.Config
	events EventLister
	people PersonFinder
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

func todayStartUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func helpText(reminderDays int) string {
	return "Datey bot — upcoming dates\n" +
		"/upcoming [days] — next N days (default " + strconv.Itoa(reminderDays) + ")\n" +
		"/today — today's events\n" +
		"/person <name> — a person's upcoming dates\n" +
		"/search <query> — find people"
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
	spaceIdx := strings.IndexFunc(text, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
	var cmdPart, argPart string
	if spaceIdx == -1 {
		cmdPart = text
		argPart = ""
	} else {
		cmdPart = text[:spaceIdx]
		argPart = strings.TrimSpace(text[spaceIdx+1:])
	}
	// Strip @bot suffix.
	if at := strings.Index(cmdPart, "@"); at != -1 {
		cmdPart = cmdPart[:at]
	}
	cmdPart = strings.ToLower(cmdPart)

	var reply string
	switch cmdPart {
	case "/start", "/help":
		reply = helpText(b.cfg.ReminderDays)
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
	return b.sender.SendMessage(ctx, chatID, text)
}
