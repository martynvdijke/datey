package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/session"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	cfg          *config.Config
	client       *ent.Client
	templates    map[string]*template.Template
	users        *repository.UserRepository
	sessions     *session.Store
	contacts     *repository.ContactRepository
	events       *repository.EventRepository
	oneTimeNots  *repository.OneTimeNotificationRepository
	notifReg     *notifier.Registry
	logStore     *logstore.Store
}

func NewHandler(cfg *config.Config, client *ent.Client, notifReg *notifier.Registry, logStore *logstore.Store) *Handler {
	templates, err := loadTemplates()
	if err != nil {
		panic(err)
	}
	return &Handler{
		cfg:         cfg,
		client:      client,
		templates:   templates,
		users:       repository.NewUserRepository(client),
		sessions:    session.NewStore(client),
		contacts:    repository.NewContactRepository(client),
		events:      repository.NewEventRepository(client),
		oneTimeNots: repository.NewOneTimeNotificationRepository(client),
		notifReg:    notifReg,
		logStore:    logStore,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	// Health check on its own (no middleware applied)
	r.Get("/health", h.healthCheck)

	// All other routes with middleware applied via group
	r.Group(func(r chi.Router) {
		r.Use(h.SetupRedirect)

		r.NotFound(h.notFound)

		// Public routes — no auth required
		r.Get("/setup", h.setupPage)
		r.Post("/setup", h.setupCreate)
		r.Get("/login", h.loginPage)
		r.Post("/login", h.loginPost)
		r.Get("/logout", h.logout)

		// Protected routes — require authentication
		r.Group(func(r chi.Router) {
			r.Use(h.Auth)

			r.Get("/", h.dashboard)
			r.Get("/contacts", h.listContacts)
			r.Get("/contacts/new", h.newContactForm)
			r.Post("/contacts/new", h.createContact)
			r.Get("/contacts/{id}", h.viewContact)
			r.Post("/contacts/{id}/delete", h.deleteContact)
			r.Post("/contacts/import", h.handleImportVCard)
			r.Get("/contacts/{id}/vcard", h.handleExportSingleVCard)
			r.Get("/contacts/export", h.handleExportAllVCard)
			r.Get("/contacts/{id}/vcard", h.handleExportSingleVCard)
			r.Get("/contacts/{id}/events/new", h.newEventForm)
			r.Post("/contacts/{id}/events/new", h.createEvent)
			r.Post("/events/{id}/delete", h.deleteEvent)
			r.Post("/contacts/import", h.handleImportVCard)
			r.Get("/contacts/export", h.handleExportAllVCard)

			r.Get("/calendar", h.calendarPage)
			r.Get("/api/calendar-events", h.calendarEvents)

			r.Get("/notifications", h.notificationsList)
			r.Get("/notifications/new", h.newNotificationForm)
			r.Post("/notifications/new", h.createNotification)
			r.Post("/notifications/{id}/delete", h.deleteNotification)
			r.Get("/api/notifications", h.apiNotifications)

			// Admin-only routes
			r.Group(func(r chi.Router) {
				r.Use(h.Admin)

				r.Get("/settings", h.settings)
				r.Get("/settings/config", h.settingsConfig)
				r.Get("/settings/logs", h.settingsLogs)
				r.Get("/settings/backup", h.settingsBackup)
				r.Post("/settings/backup", h.settingsBackupRun)
				r.Post("/settings/test/{channel}", h.testNotification)
				r.Post("/settings/logs/level", h.setLogLevel)
				// Legacy redirects
				r.Get("/logs", h.oldLogsRedirect)
				r.Post("/logs/level", h.setLogLevel)
				r.Get("/users", h.usersList)
				r.Post("/users/create", h.userCreate)
				r.Post("/users/{id}/delete", h.userDelete)
			})
		})
	})
}

func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"time":    time.Now().Format(time.RFC3339),
		"version": "1.1.2",
	})
}

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, r, http.StatusNotFound)
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	end := now.AddDate(0, 0, h.cfg.ReminderDays)

	events, err := h.events.ListUpcoming(r.Context(), now, end)
	if err != nil {
		slog.Error("dashboard: list upcoming", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
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

	h.render(w, r, "dashboard.html", map[string]any{
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
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	contactsWithEvents, err := h.client.Contact.Query().All(r.Context())
	if err != nil {
		slog.Error("list contacts with events", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
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

	h.render(w, r, "contacts.html", map[string]any{
		"Title":    "Datey - Contacts",
		"Contacts": contactsWithEvents,
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

func (h *Handler) newEventForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	h.render(w, r, "event_form.html", map[string]any{
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
		h.renderError(w, r, http.StatusInternalServerError)
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

func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	// If already authenticated, redirect to dashboard
	if u := UserFromContext(r.Context()); u != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	h.render(w, r, "login.html", map[string]any{
		"Title": "Datey - Login",
	})
}

func (h *Handler) loginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		h.render(w, r, "login.html", map[string]any{
			"Title":  "Datey - Login",
			"Error":  "Username and password are required",
		})
		return
	}

	u, err := h.users.GetByUsername(r.Context(), username)
	if err != nil {
		h.render(w, r, "login.html", map[string]any{
			"Title": "Datey - Login",
			"Error": "Invalid username or password",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		h.render(w, r, "login.html", map[string]any{
			"Title": "Datey - Login",
			"Error": "Invalid username or password",
		})
		return
	}

	token, err := h.sessions.Create(r.Context(), u.ID)
	if err != nil {
		slog.Error("login: create session", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	session.SetCookie(w, token, r.TLS != nil)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	token, err := session.ReadCookie(r)
	if err == nil && token != "" {
		h.sessions.Delete(r.Context(), token)
	}
	session.ClearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) setupPage(w http.ResponseWriter, r *http.Request) {
	exists, err := h.users.Exists(r.Context())
	if err == nil && exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.render(w, r, "setup.html", map[string]any{
		"Title": "Datey - Setup",
	})
}

func (h *Handler) setupCreate(w http.ResponseWriter, r *http.Request) {
	exists, _ := h.users.Exists(r.Context())
	if exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" {
		h.render(w, r, "setup.html", map[string]any{
			"Title": "Datey - Setup",
			"Error": "Username is required",
		})
		return
	}

	if len(password) < 8 {
		h.render(w, r, "setup.html", map[string]any{
			"Title": "Datey - Setup",
			"Error": "Password must be at least 8 characters",
		})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("setup: hash password", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	_, err = h.users.Create(r.Context(), username, string(hash), user.RoleAdmin)
	if err != nil {
		h.render(w, r, "setup.html", map[string]any{
			"Title": "Datey - Setup",
			"Error": "Failed to create admin user: " + err.Error(),
		})
		return
	}

	slog.Info("admin user created", "username", username)
	http.Redirect(w, r, "/login?success=Admin+account+created.+Please+log+in.", http.StatusSeeOther)
}

func (h *Handler) usersList(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		slog.Error("users list", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	h.render(w, r, "users.html", map[string]any{
		"Title": "Datey - Users",
		"Users": users,
	})
}

func (h *Handler) userCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	role := r.FormValue("role")

	if username == "" {
		http.Redirect(w, r, "/users?error=Username+is+required", http.StatusSeeOther)
		return
	}
	if len(password) < 8 {
		http.Redirect(w, r, "/users?error=Password+must+be+at+least+8+characters", http.StatusSeeOther)
		return
	}

	// Check for duplicate username
	existing, err := h.users.GetByUsername(r.Context(), username)
	if err == nil && existing != nil {
		http.Redirect(w, r, "/users?error=Username+"+username+"+is+already+taken", http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("create user: hash password", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	userRole := user.RoleUser
	if role == "admin" {
		userRole = user.RoleAdmin
	}

	_, err = h.users.Create(r.Context(), username, string(hash), userRole)
	if err != nil {
		slog.Error("create user", "error", err)
		http.Redirect(w, r, "/users?error=Failed+to+create+user", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/users?success=User+"+username+"+created", http.StatusSeeOther)
}

func (h *Handler) userDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	currentUser := UserFromContext(r.Context())
	if currentUser != nil && currentUser.ID == id {
		http.Redirect(w, r, "/users?error=You+cannot+delete+your+own+account", http.StatusSeeOther)
		return
	}

	// Look up the username before deleting
	userToDelete, lookupErr := h.users.GetByID(r.Context(), id)
	username := ""
	if lookupErr == nil && userToDelete != nil {
		username = userToDelete.Username
	}

	// Delete all sessions for this user first
	if err := h.sessions.DeleteByUserID(r.Context(), id); err != nil {
		slog.Error("delete user: delete sessions", "error", err)
	}

	if err := h.users.Delete(r.Context(), id); err != nil {
		slog.Error("delete user", "error", err)
		http.Redirect(w, r, "/users?error=Failed+to+delete+user", http.StatusSeeOther)
		return
	}

	if username != "" {
		http.Redirect(w, r, "/users?success=User+"+username+"+deleted", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/users?success=User+deleted", http.StatusSeeOther)
	}
}

func (h *Handler) calendarPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "calendar.html", map[string]any{
		"Title": "Datey - Calendar",
	})
}

func (h *Handler) calendarEvents(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		// Default to start of current month
		now := time.Now()
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		// Default to end of next month
		now := time.Now()
		end = time.Date(now.Year(), now.Month()+2, 0, 0, 0, 0, 0, time.UTC)
	}

	events, err := h.events.ListInRange(r.Context(), start, end)
	if err != nil {
		slog.Error("calendar events: list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Color map by event type
	colorMap := map[string]string{
		"birthday":    "#0d6efd",
		"anniversary": "#198754",
		"wedding":     "#ffc107",
		"holiday":     "#dc3545",
		"meeting":     "#6f42c1",
		"custom":      "#6c757d",
	}

	type calendarEvent struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Start       string `json:"start"`
		AllDay      bool   `json:"allDay"`
		Background  string `json:"backgroundColor"`
		Border      string `json:"borderColor"`
		TextColor   string `json:"textColor"`
		Description string `json:"description"`
		Type        string `json:"type"`
	}

	var result []calendarEvent
	for _, e := range events {
		contactName := ""
		if contact := e.Edges.Contact; contact != nil {
			contactName = contact.Name
		}
		color, ok := colorMap[e.Type]
		if !ok {
			color = "#6c757d"
		}
		title := contactName
		if e.Type != "" {
			title = contactName + " - " + e.Type
		}
		result = append(result, calendarEvent{
			ID:          fmt.Sprintf("%d", e.ID),
			Title:       title,
			Start:       e.Date.Format("2006-01-02"),
			AllDay:      true,
			Background:  color,
			Border:      color,
			TextColor:   "#ffffff",
			Description: e.Description,
			Type:        e.Type,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
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

	h.render(w, r, "settings.html", map[string]any{
		"Title":       "Datey - Settings",
		"SettingsTab": "notifications",
		"Channels":    channels,
	})
}

func (h *Handler) settingsConfig(w http.ResponseWriter, r *http.Request) {
	type configItem struct {
		Key   string
		Value string
	}

	cfg := h.cfg
	items := []configItem{
		{"Port", fmt.Sprintf("%d", cfg.Port)},
		{"DataDir", cfg.DataDir},
		{"SchedulerHour", fmt.Sprintf("%d", cfg.SchedulerHour)},
		{"ReminderDays", fmt.Sprintf("%d", cfg.ReminderDays)},
		{"LogLevel", cfg.LogLevel},
		{"LogBufferSize", fmt.Sprintf("%d", cfg.LogBufferSize)},
		{"OTLPEndpoint", maskValue(cfg.OTLPEndpoint)},
		{"SMTPHost", cfg.SMTPHost},
		{"SMTPPort", fmt.Sprintf("%d", cfg.SMTPPort)},
		{"SMTPUser", cfg.SMTPUser},
		{"SMTPPass", maskValue(cfg.SMTPPass)},
		{"SMTPTLS", fmt.Sprintf("%v", cfg.SMTPTLS)},
		{"NotifyEmail", cfg.NotifyEmail},
		{"GotifyURL", cfg.GotifyURL},
		{"GotifyToken", maskValue(cfg.GotifyToken)},
		{"TelegramBotToken", maskValue(cfg.TelegramBotToken)},
		{"TelegramChatID", cfg.TelegramChatID},
		{"UmamiURL", cfg.UmamiURL},
		{"UmamiWebsiteID", cfg.UmamiWebsiteID},
		{"BackupDir", cfg.BackupDir},
		{"BackupRetentionDays", fmt.Sprintf("%d", cfg.BackupRetentionDays)},
	}

	h.render(w, r, "settings.html", map[string]any{
		"Title":       "Datey - Settings",
		"SettingsTab": "config",
		"ConfigItems": items,
	})
}

func maskValue(s string) string {
	if s == "" {
		return "—"
	}
	return "****"
}

func (h *Handler) settingsLogs(w http.ResponseWriter, r *http.Request) {
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

	h.render(w, r, "settings.html", map[string]any{
		"Title":        "Datey - Settings",
		"SettingsTab":  "logs",
		"Entries":      entries,
		"Total":        total,
		"Page":         page,
		"Limit":        limit,
		"LevelFilter":  levelFilter,
		"SourceFilter": sourceFilter,
		"CurrentLevel": currentLevel,
	})
}

func (h *Handler) settingsBackup(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "settings.html", map[string]any{
		"Title":              "Datey - Settings",
		"SettingsTab":        "backup",
		"BackupDir":          h.cfg.BackupDir,
		"BackupRetentionDays": h.cfg.BackupRetentionDays,
	})
}

func (h *Handler) settingsBackupRun(w http.ResponseWriter, r *http.Request) {
	dbPath := h.cfg.DataDir + "/datey.db"
	if err := db.Backup(dbPath, h.cfg.BackupDir, h.cfg.BackupRetentionDays); err != nil {
		slog.Error("manual backup failed", "error", err)
		w.Write([]byte(`<div class="alert alert-danger">Backup failed: ` + err.Error() + `</div>`))
		return
	}
	slog.Info("manual backup completed", "dir", h.cfg.BackupDir)
	w.Write([]byte(`<div class="alert alert-success">Backup completed successfully!</div>`))
}

func (h *Handler) oldLogsRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/settings/logs", http.StatusMovedPermanently)
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

func (h *Handler) notificationsList(w http.ResponseWriter, r *http.Request) {
	notifications, err := h.oneTimeNots.List(r.Context())
	if err != nil {
		slog.Error("notifications list", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	h.render(w, r, "notifications.html", map[string]any{
		"Title":         "Datey - One-Time Notifications",
		"Notifications": notifications,
	})
}

type channelInfo struct {
	Name       string
	Label      string
	Configured bool
}

func (h *Handler) channelInfoList() []channelInfo {
	return []channelInfo{
		{"email", "Email", h.notifReg.IsConfigured("email")},
		{"gotify", "Gotify", h.notifReg.IsConfigured("gotify")},
		{"telegram", "Telegram", h.notifReg.IsConfigured("telegram")},
	}
}

func (h *Handler) newNotificationForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "notification_form.html", map[string]any{
		"Title":    "Datey - Create Notification",
		"Channels": h.channelInfoList(),
	})
}

func (h *Handler) createNotification(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	message := r.FormValue("message")
	scheduledAtStr := r.FormValue("scheduled_at")

	errors := make(map[string]string)

	if message == "" {
		errors["message"] = "Message is required"
	}

	if scheduledAtStr == "" {
		errors["scheduled_at"] = "Scheduled date and time is required"
	}

	if len(errors) > 0 {
		h.render(w, r, "notification_form.html", map[string]any{
			"Title":    "Datey - Create Notification",
			"Errors":   errors,
			"FormData": map[string]string{
				"Message":     message,
				"ScheduledAt": scheduledAtStr,
			},
			"Channels": h.channelInfoList(),
		})
		return
	}

	scheduledAt, err := time.ParseInLocation("2006-01-02T15:04", scheduledAtStr, time.Local)
	if err != nil {
		errors["scheduled_at"] = "Invalid date/time format"
		h.render(w, r, "notification_form.html", map[string]any{
			"Title":    "Datey - Create Notification",
			"Errors":   errors,
			"FormData": map[string]string{
				"Message":     message,
				"ScheduledAt": scheduledAtStr,
			},
			"Channels": h.channelInfoList(),
		})
		return
	}

	if scheduledAt.Before(time.Now()) {
		errors["scheduled_at"] = "Scheduled time must be in the future"
		h.render(w, r, "notification_form.html", map[string]any{
			"Title":    "Datey - Create Notification",
			"Errors":   errors,
			"FormData": map[string]string{
				"Message":     message,
				"ScheduledAt": scheduledAtStr,
			},
			"Channels": h.channelInfoList(),
		})
		return
	}

	// Parse selected channel targets (default to all configured if none selected)
	channels := r.Form["channels"]
	if len(channels) == 0 {
		for _, name := range []string{"email", "gotify", "telegram"} {
			if h.notifReg.IsConfigured(name) {
				channels = append(channels, name)
			}
		}
	}

	if len(channels) == 0 {
		errors["channels"] = "At least one notification channel must be selected"
		h.render(w, r, "notification_form.html", map[string]any{
			"Title":  "Datey - Create Notification",
			"Errors": errors,
			"FormData": map[string]string{
				"Message":     message,
				"ScheduledAt": scheduledAtStr,
			},
			"Channels": h.channelInfoList(),
		})
		return
	}

	_, err = h.oneTimeNots.Create(r.Context(), message, scheduledAt, channels)
	if err != nil {
		slog.Error("create notification", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

func (h *Handler) deleteNotification(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.oneTimeNots.Delete(r.Context(), id); err != nil {
		slog.Error("delete notification", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) apiNotifications(w http.ResponseWriter, r *http.Request) {
	notifications, err := h.oneTimeNots.List(r.Context())
	if err != nil {
		slog.Error("api notifications list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type apiNotification struct {
		ID          int        `json:"id"`
		Message     string     `json:"message"`
		ScheduledAt time.Time  `json:"scheduled_at"`
		Status      string     `json:"status"`
		CreatedAt   time.Time  `json:"created_at"`
		SentAt      *time.Time `json:"sent_at,omitempty"`
	}

	result := make([]apiNotification, len(notifications))
	for i, n := range notifications {
		result[i] = apiNotification{
			ID:          n.ID,
			Message:     n.Message,
			ScheduledAt: n.ScheduledAt,
			Status:      n.Status,
			CreatedAt:   n.CreatedAt,
			SentAt:      n.SentAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *Handler) baseData(r *http.Request, title string) map[string]any {
	umamiConfigured := h.cfg.UmamiURL != "" && h.cfg.UmamiWebsiteID != ""
	data := map[string]any{
		"Title":            title,
		"UmamiURL":         h.cfg.UmamiURL,
		"UmamiWebsiteID":   h.cfg.UmamiWebsiteID,
		"UmamiConfigured":  umamiConfigured,
	}
	u := UserFromContext(r.Context())
	if u != nil {
		data["User"] = u
		data["IsAdmin"] = u.Role == user.RoleAdmin
	}
	// Flash messages from query params (for redirect-based messages)
	if s := r.URL.Query().Get("success"); s != "" {
		data["Success"] = s
	}
	if e := r.URL.Query().Get("error"); e != "" {
		data["Error"] = e
	}
	return data
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, ok := h.templates[page]
	if !ok {
		slog.Error("template not found", "page", page)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Merge base data (Umami config, user, etc.) with page-specific data.
	title, _ := data["Title"].(string)
	merged := h.baseData(r, title)
	for k, v := range data {
		merged[k] = v
	}

	if err := tmpl.ExecuteTemplate(w, "base.html", merged); err != nil {
		slog.Error("render template", "error", err)
	}
}

func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, status int) {
	w.WriteHeader(status)
	statusText := http.StatusText(status)
	h.render(w, r, "error.html", map[string]any{
		"Title":      "Datey - " + statusText,
		"StatusCode": status,
		"StatusText": statusText,
	})
}
