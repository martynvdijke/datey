package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	cfg          *config.Config
	client       *ent.Client
	tmpl         *template.Template
	contacts     *repository.ContactRepository
	events       *repository.EventRepository
	notifReg     *notifier.Registry
}

func NewHandler(cfg *config.Config, client *ent.Client, notifReg *notifier.Registry) *Handler {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
	return &Handler{
		cfg:      cfg,
		client:   client,
		tmpl:     tmpl,
		contacts: repository.NewContactRepository(client),
		events:   repository.NewEventRepository(client),
		notifReg: notifReg,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.dashboard)
	r.Get("/contacts", h.listContacts)
	r.Get("/contacts/new", h.newContactForm)
	r.Post("/contacts/new", h.createContact)
	r.Get("/contacts/{id}", h.viewContact)
	r.Post("/contacts/{id}/delete", h.deleteContact)
	r.Get("/contacts/{id}/events/new", h.newEventForm)
	r.Post("/contacts/{id}/events/new", h.createEvent)
	r.Post("/events/{id}/delete", h.deleteEvent)
	r.Get("/settings", h.settings)
	r.Post("/settings/test/{channel}", h.testNotification)
}

func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	end := now.AddDate(0, 0, h.cfg.ReminderDays)

	events, err := h.events.ListUpcoming(r.Context(), now, end)
	if err != nil {
		slog.Error("dashboard: list upcoming", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type eventView struct {
		Name          string
		Type          string
		Date          string
		DaysRemaining int
	}

	var evs []eventView
	for _, e := range events {
		contactName := ""
		if contact := e.Edges.Contact; contact != nil {
			contactName = contact.Name
		}
		days := int(e.Date.Sub(now).Hours() / 24)
		evs = append(evs, eventView{
			Name:          contactName,
			Type:          e.Type,
			Date:          e.Date.Format("Jan 2"),
			DaysRemaining: days,
		})
	}

	h.render(w, "dashboard.html", map[string]any{
		"Events":       evs,
		"ReminderDays": h.cfg.ReminderDays,
	})
}

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
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	contactsWithEvents, err := h.client.Contact.Query().All(r.Context())
	if err != nil {
		slog.Error("list contacts with events", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	for _, c := range contacts {
		events, err := h.events.ListByContact(r.Context(), c.ID)
		if err == nil {
			for _, e := range events {
				c.Edges.Events = append(c.Edges.Events, e)
			}
		}
	}

	h.render(w, "contacts.html", map[string]any{
		"Contacts": contactsWithEvents,
	})
}

func (h *Handler) newContactForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "contact_form.html", map[string]any{})
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
		http.Error(w, "internal error", http.StatusInternalServerError)
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
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	events, err := h.events.ListByContact(r.Context(), id)
	if err != nil {
		slog.Error("list events by contact", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.render(w, "contact_detail.html", map[string]any{
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
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/contacts", http.StatusSeeOther)
}

func (h *Handler) newEventForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	h.render(w, "event_form.html", map[string]any{
		"ContactID": id,
	})
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	eventType := r.FormValue("type")
	dateStr := r.FormValue("date")
	description := r.FormValue("description")

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}

	_, err = h.events.Create(r.Context(), id, eventType, date, description)
	if err != nil {
		slog.Error("create event", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/contacts/%d", id), http.StatusSeeOther)
}

func (h *Handler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.events.Delete(r.Context(), id); err != nil {
		slog.Error("delete event", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	type channelStatus struct {
		Name       string
		Configured bool
	}

	channels := []channelStatus{
		{Name: "email", Configured: h.notifReg.IsConfigured("email")},
		{Name: "gotify", Configured: h.notifReg.IsConfigured("gotify")},
		{Name: "telegram", Configured: h.notifReg.IsConfigured("telegram")},
	}

	h.render(w, "settings.html", map[string]any{
		"Channels": channels,
	})
}

func (h *Handler) testNotification(w http.ResponseWriter, r *http.Request) {
	channel := chi.URLParam(r, "channel")

	if !h.notifReg.IsConfigured(channel) {
		http.Error(w, "channel not configured", http.StatusBadRequest)
		return
	}

	title := "Datey Test Notification"
	message := fmt.Sprintf("This is a test notification sent at %s", time.Now().Format(time.RFC3339))

	switch channel {
	case "email":
		n := notifier.NewEmailNotifier(h.cfg)
		if err := n.Send(r.Context(), title, message); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "gotify":
		n := notifier.NewGotifyNotifier(h.cfg)
		if err := n.Send(r.Context(), title, message); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "telegram":
		n := notifier.NewTelegramNotifier(h.cfg)
		if err := n.Send(r.Context(), title, message); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "unknown channel", http.StatusBadRequest)
		return
	}

	w.Write([]byte("✅ Test sent!"))
}

func (h *Handler) render(w http.ResponseWriter, _ string, data map[string]any) {
	if err := h.tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		slog.Error("render template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
