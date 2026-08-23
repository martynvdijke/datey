package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/datey/datey/ent/giftidea"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) createGiftIdea(w http.ResponseWriter, r *http.Request) {
	personID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
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
	if err := h.giftIdeas.Delete(r.Context(), giftID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/people/"+strconv.Itoa(personID)+"?success=Gift+idea+deleted", http.StatusSeeOther)
}
