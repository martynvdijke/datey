package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/datey/datey/internal/repository"
)

func (h *Handler) auditLog(w http.ResponseWriter, r *http.Request) {
	actor := r.URL.Query().Get("actor")
	action := r.URL.Query().Get("action")
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	pageStr := r.URL.Query().Get("page")
	page := 0
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	limit := 50
	offset := page * limit
	var from, to *time.Time
	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = &t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			end := t.Add(24*time.Hour - time.Nanosecond)
			to = &end
		}
	}
	repo := repository.NewAuditEntryRepository(h.client)
	filter := repository.AuditFilter{Actor: actor, Action: action, From: from, To: to, Limit: limit, Offset: offset}
	entries, err := repo.List(r.Context(), filter)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	total, _ := repo.Count(r.Context(), repository.AuditFilter{Actor: actor, Action: action, From: from, To: to})
	hasNext := (offset + limit) < total
	hasPrev := page > 0
	h.render(w, r, "audit.html", map[string]any{
		"Title":   "Datey - Audit Log",
		"Entries": entries,
		"Total":   total,
		"Page":    page,
		"HasNext": hasNext,
		"HasPrev": hasPrev,
		"Actor":   actor,
		"Action":  action,
		"From":    fromStr,
		"To":      toStr,
	})
}
