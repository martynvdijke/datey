package web

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/datey/datey/ent"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) listContacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")

	var contacts []*ent.Contact
	var err error

	if q != "" {
		contacts, err = h.contacts.Search(r.Context(), q)
	} else {
		contacts, err = h.contacts.List(r.Context())
	}

	if err != nil {
		slog.Error("list contacts", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Load events for each contact
	for _, c := range contacts {
		events, err := h.events.ListByContact(r.Context(), c.ID)
		if err == nil {
			for _, e := range events {
				c.Edges.Events = append(c.Edges.Events, e)
			}
		}
	}

	h.render(w, r, "contacts.html", map[string]any{
		"Title":    "Datey - Contacts",
		"Contacts": contacts,
	})
}

func (h *Handler) newContactForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "contact_form.html", map[string]any{
		"Title": "Datey - Add Contact",
	})
}

func (h *Handler) createContact(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	notes := r.FormValue("notes")

	_, err := h.contacts.Create(r.Context(), name, notes)
	if err != nil {
		slog.Error("create contact", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/contacts", http.StatusSeeOther)
}

func (h *Handler) viewContact(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	contact, err := h.contacts.Get(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("get contact", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	events, err := h.events.ListByContact(r.Context(), id)
	if err != nil {
		slog.Error("list events by contact", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "contact_detail.html", map[string]any{
		"Title":   "Datey - " + contact.Name,
		"Contact": contact,
		"Events":  events,
	})
}

func (h *Handler) deleteContact(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.contacts.Delete(r.Context(), id); err != nil {
		slog.Error("delete contact", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/contacts", http.StatusSeeOther)
}
