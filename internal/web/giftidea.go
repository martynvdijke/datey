package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/ent/giftidea"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) createGiftIdea(w http.ResponseWriter, r *http.Request) {
	personID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		var body struct {
			Title      string `json:"title"`
			Notes      string `json:"notes"`
			PriceCents *int   `json:"price_cents"`
			URL        string `json:"url"`
			Status     string `json:"status"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		title := strings.TrimSpace(body.Title)
		if title == "" {
			writeAPIJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation", "message": "Title is required", "fields": map[string]string{"title": "required"}})
			return
		}
		if body.URL != "" && !strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://") {
			writeAPIJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation", "message": "Invalid URL", "fields": map[string]string{"url": "must start with http"}})
			return
		}
		if body.PriceCents != nil && *body.PriceCents < 0 {
			writeAPIJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation", "message": "Invalid price", "fields": map[string]string{"price_cents": "must be >=0"}})
			return
		}
		g, err := h.giftIdeas.Create(r.Context(), personID, title, strings.TrimSpace(body.Notes), body.PriceCents, strings.TrimSpace(body.URL))
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal", "failed")
			return
		}
		// handle status if provided and not default
		if body.Status != "" && body.Status != "idea" {
			if status := giftidea.Status(body.Status); giftidea.StatusValidator(status) == nil {
				if updated, err := h.giftIdeas.UpdateStatus(r.Context(), g.ID, status); err == nil {
					g = updated
				}
			}
		}
		writeAPIJSON(w, http.StatusCreated, GiftIdeaDTO{ID: g.ID, Title: g.Title, Notes: g.Notes, PriceCents: g.PriceCents, URL: g.URL, Status: string(g.Status), CreatedAt: g.CreatedAt.Format(time.RFC3339), UpdatedAt: g.UpdatedAt.Format(time.RFC3339)})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	urlStr := strings.TrimSpace(r.FormValue("url"))
	priceStr := strings.TrimSpace(r.FormValue("price_cents"))

	if title == "" {
		http.Redirect(w, r, "/people/"+strconv.Itoa(personID)+"?error=Title+is+required", http.StatusSeeOther)
		return
	}
	if urlStr != "" && !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		http.Redirect(w, r, "/people/"+strconv.Itoa(personID)+"?error=URL+must+start+with+http", http.StatusSeeOther)
		return
	}
	var priceCents *int
	if priceStr != "" {
		p, err := strconv.Atoi(priceStr)
		if err != nil || p < 0 {
			http.Redirect(w, r, "/people/"+strconv.Itoa(personID)+"?error=Invalid+price", http.StatusSeeOther)
			return
		}
		priceCents = &p
	}
	if _, err := h.giftIdeas.Create(r.Context(), personID, title, notes, priceCents, urlStr); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/people/"+strconv.Itoa(personID)+"?success=Gift+idea+added", http.StatusSeeOther)
}

func (h *Handler) updateGiftIdeaStatus(w http.ResponseWriter, r *http.Request) {
	personID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	giftID, err := strconv.Atoi(chi.URLParam(r, "giftID"))
	if err != nil {
		http.Error(w, "invalid gift id", http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		var body struct {
			Status string `json:"status"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		status := giftidea.Status(strings.TrimSpace(body.Status))
		if err := giftidea.StatusValidator(status); err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation", "invalid status")
			return
		}
		updated, err := h.giftIdeas.UpdateStatus(r.Context(), giftID, status)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal", "failed")
			return
		}
		writeAPIJSON(w, http.StatusOK, GiftIdeaDTO{ID: updated.ID, Title: updated.Title, Notes: updated.Notes, PriceCents: updated.PriceCents, URL: updated.URL, Status: string(updated.Status), CreatedAt: updated.CreatedAt.Format(time.RFC3339), UpdatedAt: updated.UpdatedAt.Format(time.RFC3339)})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	statusStr := strings.TrimSpace(r.FormValue("status"))
	status := giftidea.Status(statusStr)
	if err := giftidea.StatusValidator(status); err != nil {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if _, err := h.giftIdeas.UpdateStatus(r.Context(), giftID, status); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/people/"+strconv.Itoa(personID)+"?success=Gift+status+updated", http.StatusSeeOther)
}

func (h *Handler) updateGiftIdea(w http.ResponseWriter, r *http.Request) {
	if !wantsJSON(r) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	personID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	giftID, err := strconv.Atoi(chi.URLParam(r, "giftID"))
	if err != nil {
		http.Error(w, "invalid gift id", http.StatusBadRequest)
		return
	}
	if u := UserFromContext(r.Context()); u != nil && u.LinkedPersonID != nil && *u.LinkedPersonID == personID {
		writeAPIError(w, http.StatusForbidden, "forbidden", "Cannot edit gifts for your own profile")
		return
	}
	var body struct {
		Title      string `json:"title"`
		Notes      string `json:"notes"`
		PriceCents *int   `json:"price_cents"`
		URL        string `json:"url"`
		Status     string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid JSON")
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		writeAPIJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation", "message": "Title is required", "fields": map[string]string{"title": "required"}})
		return
	}
	urlStr := strings.TrimSpace(body.URL)
	if urlStr != "" && !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		writeAPIJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation", "message": "Invalid URL", "fields": map[string]string{"url": "must start with http"}})
		return
	}
	if body.PriceCents != nil && *body.PriceCents < 0 {
		writeAPIJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation", "message": "Invalid price", "fields": map[string]string{"price_cents": "must be >=0"}})
		return
	}
	statusStr := strings.TrimSpace(body.Status)
	if statusStr == "" {
		statusStr = "idea"
	}
	status := giftidea.Status(statusStr)
	if err := giftidea.StatusValidator(status); err != nil {
		writeAPIJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation", "message": "Invalid status", "fields": map[string]string{"status": "invalid"}})
		return
	}
	if _, err := h.giftIdeas.Get(r.Context(), giftID); err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "Gift not found")
		return
	}
	updated, err := h.giftIdeas.Update(r.Context(), giftID, title, strings.TrimSpace(body.Notes), body.PriceCents, urlStr, status)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal", "failed")
		return
	}
	writeAPIJSON(w, http.StatusOK, GiftIdeaDTO{ID: updated.ID, Title: updated.Title, Notes: updated.Notes, PriceCents: updated.PriceCents, URL: updated.URL, Status: string(updated.Status), CreatedAt: updated.CreatedAt.Format(time.RFC3339), UpdatedAt: updated.UpdatedAt.Format(time.RFC3339)})
}

func (h *Handler) deleteGiftIdea(w http.ResponseWriter, r *http.Request) {
	personID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	giftID, err := strconv.Atoi(chi.URLParam(r, "giftID"))
	if err != nil {
		http.Error(w, "invalid gift id", http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		if err := h.giftIdeas.Delete(r.Context(), giftID); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal", "failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.giftIdeas.Delete(r.Context(), giftID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/people/"+strconv.Itoa(personID)+"?success=Gift+idea+deleted", http.StatusSeeOther)
}
