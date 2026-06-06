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
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	cfg          *config.Config
	client       *ent.Client
	templates    map[string]*template.Template
	contacts     *repository.ContactRepository
	events       *repository.EventRepository
	notifReg     *notifier.Registry
	logStore     *logstore.Store
}

func NewHandler(cfg *config.Config, client *ent.Client, notifReg *notifier.Registry, logStore *logstore.Store) *Handler {
	templates, err := loadTemplates()
	if err != nil {
		panic(err)
	}
	return &Handler{
		cfg:       cfg,
		client:    client,
		templates: templates,
		contacts:  repository.NewContactRepository(client),
		events:    repository.NewEventRepository(client),
		notifReg:  notifReg,
		logStore:  logStore,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.NotFound(h.notFound)

	r.Get("/health", h.healthCheck)
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
	r.Get("/logs", h.logsPage)
	r.Post("/logs/level", h.setLogLevel)
}

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, http.StatusNotFound)
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
		h.renderError(w, http.StatusInternalServerError)
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
		"Title":        "Datey - Dashboard",
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
		h.renderError(w, http.StatusInternalServerError)
		return
	}

	contactsWithEvents, err := h.client.Contact.Query().All(r.Context())
	if err != nil {
		slog.Error("list contacts with events", "error", err)
		h.renderError(w, http.StatusInternalServerError)
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
		"Title":    "Datey - Contacts",
		"Contacts": contactsWithEvents,
	})
}

func (h *Handler) newContactForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "contact_form.html", map[string]any{
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
		h.renderError(w, http.StatusInternalServerError)
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
		h.renderError(w, http.StatusInternalServerError)
		return
	}

	events, err := h.events.ListByContact(r.Context(), id)
	if err != nil {
		slog.Error("list events by contact", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.render(w, "contact_detail.html", map[string]any{
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
		h.renderError(w, http.StatusInternalServerError)
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
		"Title":     "Datey - Add Event",
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
		h.renderError(w, http.StatusInternalServerError)
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
		"Title":    "Datey - Settings",
		"Channels": channels,
	})
}

func (h *Handler) testNotification(w http.ResponseWriter, r *http.Request) {
	channel := chi.URLParam(r, "channel")

	if !h.notifReg.IsConfigured(channel) {
		slog.Warn("test notification: channel not configured", "source", "settings", "channel", channel)
		http.Error(w, "channel not configured", http.StatusBadRequest)
		return
	}

	title := "Datey Test Notification"
	message := fmt.Sprintf("This is a test notification sent at %s", time.Now().Format(time.RFC3339))

	var err error
	switch channel {
	case "email":
		n := notifier.NewEmailNotifier(h.cfg)
		err = n.Send(r.Context(), title, message)
	case "gotify":
		n := notifier.NewGotifyNotifier(h.cfg)
		err = n.Send(r.Context(), title, message)
	case "telegram":
		n := notifier.NewTelegramNotifier(h.cfg)
		err = n.Send(r.Context(), title, message)
	default:
		slog.Warn("test notification: unknown channel", "source", "settings", "channel", channel)
		http.Error(w, "unknown channel", http.StatusBadRequest)
		return
	}

	if err != nil {
		slog.Error("test notification failed", "source", "settings", "channel", channel, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("test notification sent", "source", "settings", "channel", channel)
	w.Write([]byte("✅ Test sent!"))
}

func (h *Handler) logsPage(w http.ResponseWriter, r *http.Request) {
	levelFilter := r.URL.Query().Get("level")
	sourceFilter := r.URL.Query().Get("source")

	pageStr := r.URL.Query().Get("page")
	page := 0
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	limit := 100
	offset := page * limit

	entries, total := h.logStore.Query(levelFilter, sourceFilter, offset, limit)

	currentLevel := logstore.LevelName(h.logStore.Level())

	h.render(w, "logs.html", map[string]any{
		"Title":        "Datey - Logs",
		"Entries":      entries,
		"Total":        total,
		"Page":         page,
		"Limit":        limit,
		"LevelFilter":  levelFilter,
		"SourceFilter": sourceFilter,
		"CurrentLevel": currentLevel,
	})
}

func (h *Handler) setLogLevel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	level, ok := logstore.ParseLogLevel(req.Level)
	if !ok {
		http.Error(w, "invalid level, use: debug, info, warn, error", http.StatusBadRequest)
		return
	}

	prev := logstore.LevelName(h.logStore.Level())
	h.logStore.SetLevel(level)

	slog.Info("log level changed", "from", prev, "to", req.Level)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"level": req.Level,
	})
}

func (h *Handler) baseData(title string) map[string]any {
	umamiConfigured := h.cfg.UmamiURL != "" && h.cfg.UmamiWebsiteID != ""
	return map[string]any{
		"Title":            title,
		"UmamiURL":         h.cfg.UmamiURL,
		"UmamiWebsiteID":   h.cfg.UmamiWebsiteID,
		"UmamiConfigured":  umamiConfigured,
	}
}

func (h *Handler) render(w http.ResponseWriter, page string, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, ok := h.templates[page]
	if !ok {
		slog.Error("template not found", "page", page)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Merge base data (Umami config, etc.) with page-specific data.
	title, _ := data["Title"].(string)
	merged := h.baseData(title)
	for k, v := range data {
		merged[k] = v
	}

	if err := tmpl.ExecuteTemplate(w, "base.html", merged); err != nil {
		slog.Error("render template", "error", err)
	}
}

func (h *Handler) renderError(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
	statusText := http.StatusText(status)
	h.render(w, "error.html", map[string]any{
		"Title":      "Datey - " + statusText,
		"StatusCode": status,
		"StatusText": statusText,
	})
}
