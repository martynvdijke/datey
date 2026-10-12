package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/datey/datey/internal/telegram"
)

type telegramUpdateHandler interface {
	HandleUpdate(ctx context.Context, upd *telegram.Update)
}

func (h *Handler) SetTelegramHandler(th telegramUpdateHandler) { h.telegram = th }

func (h *Handler) telegramWebhook(w http.ResponseWriter, r *http.Request) {
	if h.cfg.TelegramBotToken == "" || h.cfg.TelegramWebhookSecret == "" || h.telegram == nil {
		http.NotFound(w, r)
		return
	}
	got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(h.cfg.TelegramWebhookSecret)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var upd telegram.Update
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&upd); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	h.telegram.HandleUpdate(ctx, &upd)
	w.WriteHeader(http.StatusOK)
}
