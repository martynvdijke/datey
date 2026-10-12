package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/telegram"
	"github.com/datey/datey/internal/web"
)

type fakeTG struct{ got []*telegram.Update }

func (f *fakeTG) HandleUpdate(ctx context.Context, upd *telegram.Update) { f.got = append(f.got, upd) }

func newWebhookRouter(t *testing.T, token, secret string) (http.Handler, *fakeTG) {
	t.Helper()
	cfg := &config.Config{
		DataDir:                    testDataDir,
		LogLevel:                   "debug",
		LogBufferSize:              100,
		BackupDir:                  testDataDir + "/backups",
		BackupRetentionDays:        30,
		TelegramBotToken:           token,
		TelegramWebhookSecret:      secret,
		ReminderDays:               7,
		DateVariant:                "european",
		ICalDurationMinutes:        60,
		NtfyPriority:               3,
		CarddavDeletePolicy:        "keep",
		GoogleDeletePolicy:         "keep",
		AuditRetentionMax:          100,
		SMTPPort:                   587,
		SMTPTimeout:                10,
		SchedulerHour:              8,
		WeeklyBackupDay:            0,
		WeeklyBackupRetentionWeeks: 52,
	}
	h := web.NewHandler(cfg, testClient, notifier.NewRegistry(), logstore.NewStore(100))
	f := &fakeTG{}
	h.SetTelegramHandler(f)
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r, f
}

func TestTelegramWebhookDisabled(t *testing.T) {
	router, f := newWebhookRouter(t, "", "")
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook", strings.NewReader(`{"update_id":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if len(f.got) != 0 {
		t.Fatalf("expected 0 calls, got %d", len(f.got))
	}
}

func TestTelegramWebhookBadSecret(t *testing.T) {
	router, f := newWebhookRouter(t, "tok", "s3cr3t")
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook", strings.NewReader(`{"update_id":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%q", rec.Code, rec.Body.String())
	}
	if len(f.got) != 0 {
		t.Fatalf("expected 0 calls, got %d", len(f.got))
	}
}

func TestTelegramWebhookDispatches(t *testing.T) {
	router, f := newWebhookRouter(t, "tok", "s3cr3t")
	body := `{"update_id":1,"message":{"message_id":2,"chat":{"id":5},"text":"/help"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "s3cr3t")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if len(f.got) != 1 {
		t.Fatalf("expected 1 call, got %d", len(f.got))
	}
	if f.got[0].UpdateID != 1 {
		t.Fatalf("expected update_id 1, got %d", f.got[0].UpdateID)
	}
	if f.got[0].Message == nil || f.got[0].Message.Text != "/help" {
		t.Fatalf("unexpected message: %+v", f.got[0].Message)
	}
	if f.got[0].Message.Chat == nil || f.got[0].Message.Chat.ID != 5 {
		t.Fatalf("unexpected chat: %+v", f.got[0].Message.Chat)
	}
}
