package web

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/handlers"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/session"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	cfg         *config.Config
	client      *ent.Client
	templates   map[string]*template.Template
	users       *repository.UserRepository
	sessions    *session.Store
	contacts    *repository.ContactRepository
	people      *repository.PersonRepository
	groups      *repository.GroupRepository
	events      *repository.EventRepository
	oneTimeNots *repository.OneTimeNotificationRepository
	notifReg    *notifier.Registry
	logStore    *logstore.Store
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
		people:      repository.NewPersonRepository(client),
		groups:      repository.NewGroupRepository(client),
		events:      repository.NewEventRepository(client),
		oneTimeNots: repository.NewOneTimeNotificationRepository(client),
		notifReg:    notifReg,
		logStore:    logStore,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	// Static files — no middleware applied
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = r.PathValue("*")
		http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
	})

	// Health check on its own (no middleware applied)
	r.Get("/health", handlers.HealthCheck)

	// Database health check (includes DB connectivity test)
	r.Get("/health/db", handlers.DBHealthCheck(h.client))

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
			// People routes (new path)
			r.Get("/people", h.listPeople)
			r.Get("/people/new", h.newPersonForm)
			r.Post("/people/new", h.createPerson)
			r.Get("/people/{id}", h.viewPerson)
			r.Post("/people/{id}/delete", h.deletePerson)
			r.Post("/people/import", h.handleImportVCard)
			r.Get("/people/{id}/vcard", h.handleExportSingleVCard)
			r.Get("/people/export", h.handleExportAllVCard)
			r.Get("/people/{id}/events/new", h.newEventForm)
			r.Post("/people/{id}/events/new", h.createEvent)
			r.Post("/events/{id}/delete", h.deleteEvent)

			// Legacy /contacts/* 301 redirects → /people/*
			r.Get("/contacts", h.redirectContactsList)
			r.Get("/contacts/new", h.redirectContactsNew)
			r.Get("/contacts/{id}", h.redirectContactsView)
			r.Get("/contacts/{id}/events/new", h.redirectContactsView)
			r.Get("/contacts/{id}/vcard", h.redirectContactsView)
			r.Get("/contacts/export", h.handleExportAllVCard)

			// Group routes (admin-only)
			r.Get("/groups", h.listGroups)
			r.Post("/groups/create", h.createGroup)
			r.Post("/groups/{id}/delete", h.deleteGroup)

			r.Get("/calendar", h.calendarPage)
			r.Get("/api/calendar-events", h.calendarEvents)

			r.Get("/notifications", h.notificationsList)
			r.Get("/notifications/new", h.newNotificationForm)
			r.Post("/notifications/new", h.createNotification)
			r.Post("/notifications/{id}/delete", h.deleteNotification)
			r.Get("/api/notifications", h.apiNotifications)

			r.Get("/roll", h.diceRoll)

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

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, r, http.StatusNotFound)
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	reminderDays := h.cfg.ReminderDays

	// Allow query param override for date finder
	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if days, err := strconv.Atoi(daysStr); err == nil && days >= 1 && days <= 365 {
			reminderDays = days
		}
	}

	end := now.AddDate(0, 0, reminderDays)

	events, err := h.events.ListUpcoming(r.Context(), now, end)
	if err != nil {
		slog.Error("dashboard: list upcoming", "error", err, "from", now.Format(time.RFC3339), "to", end.Format(time.RFC3339))
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	slog.Info("dashboard: upcoming events", "count", len(events), "from", now.Format("2006-01-02"), "to", end.Format("2006-01-02"), "reminder_days", reminderDays)

	type eventView struct {
		Name          string
		Type          string
		Date          string
		DaysRemaining int
	}

	var evs []eventView
	for _, e := range events {
		personName := ""
		if p := e.Edges.Person; p != nil {
			personName = p.Name
		} else if c := e.Edges.Contact; c != nil {
			personName = c.Name
		}
		days := int(e.Date.Sub(now).Hours() / 24)
		evs = append(evs, eventView{
			Name:          personName,
			Type:          e.Type,
			Date:          e.Date.Format("Jan 2"),
			DaysRemaining: days,
		})
	}

	h.render(w, r, "dashboard.html", map[string]any{
		"Title":        "Datey - Dashboard",
		"Events":       evs,
		"ReminderDays": reminderDays,
	})
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

func (h *Handler) baseData(r *http.Request, title string) map[string]any {
	umamiConfigured := h.cfg.UmamiURL != "" && h.cfg.UmamiWebsiteID != ""
	data := map[string]any{
		"Title":           title,
		"UmamiURL":        h.cfg.UmamiURL,
		"UmamiWebsiteID":  h.cfg.UmamiWebsiteID,
		"UmamiConfigured": umamiConfigured,
		"ActiveNav":       inferActiveNav(r.URL.Path),
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

// inferActiveNav determines which nav item should be highlighted based on the URL path.
func inferActiveNav(path string) string {
	switch {
	case path == "/" || path == "":
		return "dashboard"
	case hasPrefix(path, "/people"):
		return "people"
	case hasPrefix(path, "/groups"):
		return "groups"
	case hasPrefix(path, "/calendar") || hasPrefix(path, "/api/calendar"):
		return "calendar"
	case hasPrefix(path, "/notifications"):
		return "notifications"
	case path == "/roll":
		return "roll"
	case hasPrefix(path, "/settings") || hasPrefix(path, "/logs") || hasPrefix(path, "/users"):
		return "settings"
	default:
		return ""
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
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
	maps.Copy(merged, data)

	if err := tmpl.ExecuteTemplate(w, "base.html", merged); err != nil {
		slog.Error("render template", "error", err)
	}
}

// toastHeader sets the HX-Trigger response header to trigger a toast notification on the client.
func toastHeader(w http.ResponseWriter, message, toastType string) {
	payload := map[string]any{
		"show-toast": map[string]string{
			"message": message,
			"type":    toastType,
		},
	}
	b, _ := json.Marshal(payload)
	w.Header().Set("HX-Trigger", string(b))
}

func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, status int) {
	w.WriteHeader(status)
	statusText := http.StatusText(status)
	h.render(w, r, "error.html", map[string]any{
		"Title":      "Datey - " + statusText,
		"StatusCode": status,
		"StatusText": statusText,
	})
	// Trigger error toast for HTMX requests
	if r.Header.Get("HX-Request") == "true" {
		toastHeader(w, statusText, "error")
	}
}

func (h *Handler) diceRoll(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "roll.html", map[string]any{
		"Title": "Datey - Dice Roller",
	})
}
