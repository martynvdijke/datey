package web

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/i18n"
	"github.com/datey/datey/internal/immich"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/milestone"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/photos"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/session"
	"github.com/datey/datey/internal/settings"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	cfg                 *config.Config
	client              *ent.Client
	templates           map[string]*template.Template
	users               *repository.UserRepository
	sessions            *session.Store
	people              *repository.PersonRepository
	groups              *repository.GroupRepository
	tags                *repository.TagRepository
	events              *repository.EventRepository
	groupNotes          *repository.GroupNoteRepository
	giftIdeas           *repository.GiftIdeaRepository
	relationships       *repository.RelationshipRepository
	notifReg            *notifier.Registry
	recurringRules      *repository.RecurringRuleRepository
	pushSubs            *repository.PushSubscriptionRepository
	logStore            *logstore.Store
	settingsStore       *settings.Store
	loginLimiter        *rateLimiter
	forgotLimiter       *rateLimiter
	passwordResetTokens *repository.PasswordResetTokenRepository
	apiTokens           *repository.ApiTokenRepository
	immich              *immich.Client
	photoStore          *photos.Store
	audit               auditRecorder
}

type auditRecorder interface {
	Record(r *http.Request, action, target string)
	RecordWithActor(ctx context.Context, actor, ip, action, target string)
}

func (h *Handler) auditRecord(r *http.Request, action, target string) {
	if h.audit != nil {
		h.audit.Record(r, action, target)
	}
}

func (h *Handler) SetAuditRecorder(a auditRecorder) { h.audit = a }

func NewHandler(cfg *config.Config, client *ent.Client, notifReg *notifier.Registry, logStore *logstore.Store) *Handler {
	templates, err := loadTemplates()
	if err != nil {
		panic(err)
	}
	return &Handler{
		cfg:                 cfg,
		client:              client,
		templates:           templates,
		users:               repository.NewUserRepository(client),
		sessions:            session.NewStore(client),
		people:              repository.NewPersonRepository(client),
		groups:              repository.NewGroupRepository(client),
		tags:                repository.NewTagRepository(client),
		events:              repository.NewEventRepository(client),
		groupNotes:          repository.NewGroupNoteRepository(client),
		giftIdeas:           repository.NewGiftIdeaRepository(client),
		relationships:       repository.NewRelationshipRepository(client),
		notifReg:            notifReg,
		recurringRules:      repository.NewRecurringRuleRepository(client),
		pushSubs:            repository.NewPushSubscriptionRepository(client),
		logStore:            logStore,
		settingsStore:       settings.New(client),
		loginLimiter:        newRateLimiter(5, 60*time.Second),
		forgotLimiter:       newRateLimiter(5, 15*time.Minute),
		passwordResetTokens: repository.NewPasswordResetTokenRepository(client),
		apiTokens:           repository.NewApiTokenRepository(client),
		immich:              immich.New(cfg.ImmichURL, cfg.ImmichAPIKey),
		photoStore:          photos.NewStore(cfg.DataDir),
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	// Static files — no middleware applied
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.PathValue("*")
		if path == "manifest.json" {
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		r.URL.Path = "static/" + path
		http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
	})

	// Manifest at root for installability (spec: /manifest.json)
	r.Get("/manifest.json", h.manifest)

	// Service worker for web push — must live at the origin root for scope.
	r.Get("/sw.js", h.pushServiceWorker)

	// Health check on its own (no middleware applied)
	r.Get("/health", handlers.HealthCheck)

	// Database health check (includes DB connectivity test)
	r.Get("/health/db", handlers.DBHealthCheck(h.client))

	// All other routes with middleware applied via group
	r.Group(func(r chi.Router) {
		r.Use(h.SetupRedirect)
		r.Use(h.BearerAuth) // must run before CSRF so bearer requests are marked
		r.Use(h.CSRF)
		r.Use(h.Locale)

		r.NotFound(h.notFound)

		// Public routes — no auth required
		r.Get("/setup", h.setupPage)
		r.Post("/setup", h.setupCreate)
		r.Get("/login", h.loginPage)
		r.Post("/login", h.loginPost)
		r.Get("/logout", h.logout)
		r.Get("/forgot-password", h.forgotPasswordPage)
		r.Post("/forgot-password", h.forgotPasswordPost)
		r.Get("/reset-password", h.resetPasswordPage)
		r.Post("/reset-password", h.resetPasswordPost)

		// Public iCal feed — unauthenticated, protected by a secret key
		// (?key=...) checked inside the handlers. Feed disabled → 404.
		r.Get("/ical.ics", h.icalFeedGlobal)
		r.Get("/ical/{personID}.ics", h.icalFeedPerson)

		// Public RSS feed of upcoming events — same key protection.
		r.Get("/rss.xml", h.rssFeed)

		// Public JSON API of upcoming events — same key protection.
		r.Get("/api/upcoming", h.upcomingAPI)

		// Public Home Assistant stats feed — same key protection.
		r.Get("/api/homeassistant/stats", h.homeAssistantStats)
		r.Get("/api/homeassistant/calendar", h.homeAssistantCalendar)

		// Public TRMNL e-ink stats feed — unauthenticated (TRMNL devices cannot log in)
		r.Get("/api/trmnl/stats", h.trmnlStats)

		// Public TRMNL e-ink birthdays feed — unauthenticated (TRMNL devices cannot log in)
		r.Get("/api/trmnl/birthdays", h.trmnlBirthdays)

		// Protected routes — require authentication
		r.Group(func(r chi.Router) {
			r.Use(h.Auth)
			r.Use(h.Locale)

			r.Get("/", h.dashboard)
			// People routes (new path)
			r.Get("/people", h.listPeople)
			r.Get("/people/new", h.newPersonForm)
			r.Post("/people/new", h.createPerson)
			r.Get("/people/{id}", h.viewPerson)
			r.Get("/people/{id}/edit", h.editPersonForm)
			r.Post("/people/{id}/edit", h.updatePerson)
			r.Post("/people/{id}/delete", h.deletePerson)
			r.Post("/people/{id}/notify-birthdays", h.toggleNotifyBirthdays)
			r.Post("/people/{id}/tags", h.addPersonTag)
			r.Post("/people/{id}/tags/{tag}/remove", h.removePersonTag)
			r.Post("/people/{id}/tags/remove", h.removePersonTag)
			r.Post("/people/{id}/relationships", h.addRelationship)
			r.Post("/people/{id}/relationships/{relID}/delete", h.removeRelationship)
			r.Get("/api/tags", h.autocompleteTags)
			r.Get("/people/{id}/photo", h.personPhoto)
			r.Post("/people/{id}/immich", h.setImmichPhoto)
			r.Post("/people/{id}/photo/upload", h.uploadPersonPhoto)
			r.Post("/people/{id}/photo/remove", h.removePersonPhoto)
			r.Post("/people/{id}/gift-ideas", h.createGiftIdea)
			r.Post("/people/{id}/gift-ideas/{giftID}/status", h.updateGiftIdeaStatus)
			r.Post("/people/{id}/gift-ideas/{giftID}/delete", h.deleteGiftIdea)
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
			r.Get("/groups/export", h.handleExportGroups)
			r.Get("/groups/{id}", h.viewGroup)
			r.Get("/groups/{id}/export", h.handleExportSingleGroup)
			r.Post("/groups/{id}/delete", h.deleteGroup)
			r.Post("/groups/{id}/members", h.setGroupMembers)
			r.Post("/groups/{id}/members/add", h.addGroupMember)
			r.Post("/groups/{id}/members/{personID}/remove", h.removeGroupMember)
			r.Get("/groups/{id}/events/new", h.newGroupEventForm)
			r.Post("/groups/{id}/events/new", h.createGroupEvent)
			r.Post("/groups/{id}/notes", h.createGroupNote)
			r.Post("/groups/{id}/notes/{noteID}/delete", h.deleteGroupNote)

			r.Get("/recurring-rules", h.listRecurringRules)
			r.Get("/recurring-rules/new", h.newRecurringRuleForm)
			r.Post("/recurring-rules/new", h.createRecurringRule)
			r.Post("/recurring-rules/{id}/delete", h.deleteRecurringRule)

			r.Get("/stats", h.statsPage)
			r.Get("/calendar", h.calendarPage)
			r.Get("/api/calendar-events", h.calendarEvents)
			r.Post("/calendar/import", h.handleImportICS)
			r.Post("/calendar/import/confirm", h.handleConfirmICS)

			// E-Ink toggle: requires auth (not admin-only, any user can toggle)
			r.Post("/settings/eink-toggle", h.settingsEinkToggle)
			r.Post("/settings/locale", h.settingsLocaleSave)

			// Web Push subscription management (authenticated, CSRF-protected)
			r.Get("/push/vapid-public-key", h.pushVAPIDPublicKey)
			r.Post("/push/subscribe", h.pushSubscribe)
			r.Post("/push/unsubscribe", h.pushUnsubscribe)
			r.Get("/settings/notifications", h.settingsNotifications)
			r.Post("/settings/notifications", h.settingsNotificationsSave)
			r.Post("/settings/test/{channel}", h.testNotification)

			// API token management — owner-scoped, any authenticated user.
			// Secrets are shown exactly once at creation/rotation time.
			r.Get("/settings/api-tokens", h.apiTokensPage)
			r.Post("/settings/api-tokens", h.apiTokenCreate)
			r.Post("/settings/api-tokens/{id}/revoke", h.apiTokenRevoke)
			r.Post("/settings/api-tokens/{id}/rotate", h.apiTokenRotate)

			// Admin-only routes
			r.Group(func(r chi.Router) {
				r.Use(h.Admin)

				r.Get("/settings", h.settings)
				r.Get("/settings/config", h.settingsConfig)
				r.Post("/settings/config", h.settingsConfigSave)
				r.Get("/settings/logs", h.settingsLogs)
				r.Get("/settings/backup", h.settingsBackup)
				r.Post("/settings/backup", h.settingsBackupRun)
				r.Post("/settings/carddav", h.settingsCarddavSave)
				r.Post("/settings/carddav/sync", h.settingsCarddavSync)
				r.Post("/settings/google", h.settingsGoogleSave)
				r.Get("/settings/google/auth", h.settingsGoogleAuth)
				r.Get("/settings/google/callback", h.settingsGoogleCallback)
				r.Post("/settings/google/sync", h.settingsGoogleSync)
				r.Post("/settings/config/test/{section}", h.testConfig)
				r.Post("/settings/carddav/test", h.testCarddavConnection)
				r.Post("/settings/immich/sync", h.immichBulkSync)
				r.Post("/settings/logs/level", h.setLogLevel)
				// Legacy redirects
				r.Get("/logs", h.oldLogsRedirect)
				r.Post("/logs/level", h.setLogLevel)
				r.Get("/users", h.usersList)
				r.Post("/users/create", h.userCreate)
				r.Post("/users/{id}/delete", h.userDelete)
				r.Get("/settings/audit", h.auditLog)
				r.Post("/settings/feed/regenerate/ical", h.regenerateFeedKey("ical", "feedkey.regenerate"))
				r.Post("/settings/feed/regenerate/rss", h.regenerateFeedKey("rss", "feedkey.regenerate"))
				r.Post("/settings/feed/regenerate/upcoming", h.regenerateFeedKey("upcoming", "feedkey.regenerate"))
				r.Post("/settings/feed/regenerate/homeassistant", h.regenerateFeedKey("homeassistant", "feedkey.regenerate"))
				r.Post("/settings/feed/regenerate/trmnl", h.regenerateFeedKey("trmnl", "feedkey.regenerate"))
				r.Post("/settings/backup/restore", h.settingsBackupRestore)
				r.Get("/settings/backup/download", h.settingsBackupDownload)
			})
		})
	})
}

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, r, http.StatusNotFound)
}

// todayStartUTC returns the start of today (midnight UTC), matching how event
// dates and annual occurrences are stored/computed. "Upcoming" windows built
// from it include events dated today, which a window starting at time.Now()
// would otherwise skip (today's midnight-UTC date is earlier than the pass
// time).
func todayStartUTC() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
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

	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), todayStartUTC(), end)
	if err != nil {
		slog.Error("dashboard: list upcoming", "error", err, "from", now.Format(time.RFC3339), "to", end.Format(time.RFC3339))
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	slog.Info("dashboard: upcoming events", "count", len(occurrences), "from", now.Format("2006-01-02"), "to", end.Format("2006-01-02"), "reminder_days", reminderDays)

	// ── eventView with person context ──
	type eventView struct {
		Name           string // person name
		Type           string
		Date           string // absolute date (e.g. "Dec 25")
		DaysRemaining  int
		RelativeLabel  string // "Today", "Tomorrow", "In 3 days", or empty
		PersonInitial  string // first character for avatar
		AvatarColor    int    // deterministic colour index 0-7
		AgeInfo        age.Info
		MilestoneLabel string
	}

	var (
		todayEvents     []eventView
		thisWeekEvents  []eventView
		thisMonthEvents []eventView
		laterEvents     []eventView
	)

	// viewFor converts an occurrence into its display shape. Shared by the
	// main loop and the "always show the next birthday" fallback.
	viewFor := func(occ repository.EventOccurrence) eventView {
		e := occ.Event
		personName := eventOwnerName(e)

		days := int(occ.Date.Sub(now).Hours() / 24)

		// Relative label
		var relativeLabel string
		switch {
		case days <= 0:
			relativeLabel = "Today"
		case days == 1:
			relativeLabel = "Tomorrow"
		case days <= 7:
			relativeLabel = fmt.Sprintf("In %d days", days)
		}

		ev := eventView{
			Name:          personName,
			Type:          e.Type,
			Date:          shortDate(h.cfg.DateVariant, occ.Date),
			DaysRemaining: days,
			RelativeLabel: relativeLabel,
			PersonInitial: personInitial(personName),
			AvatarColor:   avatarColorIndex(personName),
		}
		if e.Type == "birthday" {
			// Age counts completed lunar years for lunar birthdays (one per anniversary of the stored lunar month/day).
			if e.CalendarSystem == "lunar" && e.LunarMonth != nil && e.LunarDay != nil {
				if e.Date.Year() > 1 {
					occ, _ := displayDateForEvent(e, now)
					years := now.Year() - e.Date.Year()
					nowDay := dateOnly(now, now.Location())
					occDay := dateOnly(occ, time.UTC)
					if occDay.After(nowDay) {
						years--
					}
					if years >= 0 {
						ev.AgeInfo = age.Info{Current: years, Next: years + 1, HasAge: true}
					}
				}
			} else {
				ev.AgeInfo = age.InfoFor(e.Date, now)
			}
		}
		if ok, label := milestone.IsMilestone(e.Type, e.Date, occ.Date); ok {
			ev.MilestoneLabel = label
		}
		return ev
	}

	for _, occ := range occurrences {
		days := int(occ.Date.Sub(now).Hours() / 24)

		ev := viewFor(occ)

		// Group by time horizon
		switch {
		case days <= 1:
			todayEvents = append(todayEvents, ev)
		case days <= 7:
			thisWeekEvents = append(thisWeekEvents, ev)
		case days <= 30:
			thisMonthEvents = append(thisMonthEvents, ev)
		default:
			laterEvents = append(laterEvents, ev)
		}
	}

	// Always show at least the next upcoming birthday: when no birthday falls
	// inside the reminder window, pin the next birthday — however far out —
	// to the "later" bucket so the homepage never hides it.
	hasBirthday := false
	for _, occ := range occurrences {
		if occ.Event.Type == "birthday" {
			hasBirthday = true
			break
		}
	}
	if !hasBirthday {
		if nextBirthday, err := h.events.NextBirthdayOccurrence(r.Context(), now); err != nil {
			slog.Error("dashboard: next birthday", "error", err)
		} else if nextBirthday != nil {
			laterEvents = append(laterEvents, viewFor(*nextBirthday))
		}
	}

	// ── Greeting ──
	greeting := greetingForTime(now)

	// ── Quick-glance stats ──
	allPeople, _ := h.people.List(r.Context())
	peopleCount := len(allPeople)
	totalEvents := len(todayEvents) + len(thisWeekEvents) + len(thisMonthEvents) + len(laterEvents)

	// Milestones this year summary
	var milestonesThisYear []eventView
	for _, ev := range append(append(append(todayEvents, thisWeekEvents...), thisMonthEvents...), laterEvents...) {
		if ev.MilestoneLabel != "" {
			milestonesThisYear = append(milestonesThisYear, ev)
		}
	}

	h.render(w, r, "dashboard.html", map[string]any{
		"Title":              "Datey - Dashboard",
		"Greeting":           greeting,
		"CurrentDate":        weekdayDate(h.cfg.DateVariant, now),
		"TodayEvents":        todayEvents,
		"ThisWeekEvents":     thisWeekEvents,
		"ThisMonthEvents":    thisMonthEvents,
		"LaterEvents":        laterEvents,
		"MilestonesThisYear": milestonesThisYear,
		"ReminderDays":       reminderDays,
		"PeopleCount":        peopleCount,
		"TotalEvents":        totalEvents,
	})
}

// personInitial returns the first character of a name, uppercased.
func personInitial(name string) string {
	if name == "" {
		return "?"
	}
	return string([]rune(name[:1])[0])
}

// avatarColorIndex deterministically maps a name to a colour index 0-7.
// Uses a simple FNV-like hash so the same name always gets the same colour.
func avatarColorIndex(name string) int {
	if name == "" {
		return 0
	}
	h := 0
	for _, b := range []byte(name) {
		h = h*31 + int(b)
	}
	idx := h % 8
	if idx < 0 {
		idx = -idx
	}
	return idx
}

// greetingForTime returns a time-of-day greeting.
func greetingForTime(t time.Time) string {
	hour := t.Hour()
	switch {
	case hour < 12:
		return "Good morning"
	case hour < 18:
		return "Good afternoon"
	default:
		return "Good evening"
	}
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

	errors := make(map[string]string)
	if username == "" {
		errors["username"] = "Username is required"
	}
	if len(password) < 8 {
		errors["password"] = "Password must be at least 8 characters"
	}

	// Check for duplicate username
	if username != "" {
		existing, err := h.users.GetByUsername(r.Context(), username)
		if err == nil && existing != nil {
			errors["username"] = "Username '" + username + "' is already taken"
		}
	}

	if len(errors) > 0 {
		users, err := h.users.List(r.Context())
		if err != nil {
			slog.Error("users list", "error", err)
			h.renderError(w, r, http.StatusInternalServerError)
			return
		}
		h.render(w, r, "users.html", map[string]any{
			"Title":  "Datey - Users",
			"Users":  users,
			"Errors": errors,
			"FormData": map[string]string{
				"Username": username,
				"Role":     role,
			},
		})
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
		users, listErr := h.users.List(r.Context())
		if listErr != nil {
			h.renderError(w, r, http.StatusInternalServerError)
			return
		}
		h.render(w, r, "users.html", map[string]any{
			"Title":  "Datey - Users",
			"Users":  users,
			"Errors": map[string]string{"username": "Failed to create user"},
			"FormData": map[string]string{
				"Username": username,
				"Role":     role,
			},
		})
		return
	}

	h.auditRecord(r, "user.create", username)
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
		toastHeader(w, "You cannot delete your own account", "error")
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
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
		toastHeader(w, "Failed to delete user", "error")
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
		return
	}

	if username != "" {
		toastHeader(w, "User "+username+" deleted", "success")
	} else {
		toastHeader(w, "User deleted", "success")
	}
	h.auditRecord(r, "user.delete", username)
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) baseData(r *http.Request, title string) map[string]any {
	umamiConfigured := h.cfg.UmamiURL != "" && h.cfg.UmamiWebsiteID != ""
	einkMode := h.einkModeEnabled(r)
	data := map[string]any{
		"Title":           title,
		"UmamiURL":        h.cfg.UmamiURL,
		"UmamiWebsiteID":  h.cfg.UmamiWebsiteID,
		"UmamiConfigured": umamiConfigured,
		"ActiveNav":       inferActiveNav(r.URL.Path),
		"EinkMode":        einkMode,
		"EinkForced":      h.cfg.EinkMode,
		"CSRFToken":       csrfTokenFromContext(r.Context()),
		"PushConfigured":  h.notifReg.IsConfigured("webpush"),
		"EmailConfigured": h.notifReg.IsConfigured("email"),
		"Locale":          localeFromRequest(r),
	}
	u := UserFromContext(r.Context())
	if u != nil {
		data["User"] = u
		data["IsAdmin"] = u.Role == user.RoleAdmin
	}
	// Flash messages from query params (for redirect-based messages).
	// Go's html/template auto-escapes these values when rendered in templates,
	// preventing XSS from user-crafted URLs (e.g. /login?success=<script>).
	if s := r.URL.Query().Get("success"); s != "" {
		data["Success"] = s
	}
	if e := r.URL.Query().Get("error"); e != "" {
		data["Error"] = e
	}
	return data
}

// einkModeEnabled checks if e-ink mode should be active.
// Returns true if the EINK_MODE env var is set, otherwise checks user preference.
func (h *Handler) einkModeEnabled(r *http.Request) bool {
	if h.cfg.EinkMode {
		return true
	}
	u := UserFromContext(r.Context())
	if u == nil {
		return false
	}
	enabled, err := h.users.GetEinkMode(r.Context(), u.ID)
	if err != nil {
		return false
	}
	return enabled
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
	case path == "/stats" || hasPrefix(path, "/stats"):
		return "stats"
	case hasPrefix(path, "/settings") || hasPrefix(path, "/logs") || hasPrefix(path, "/users"):
		return "settings"
	default:
		return ""
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func localeFromRequest(r *http.Request) string {
	return i18n.LocaleFromContext(r.Context())
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

	locale := i18n.LocaleFromContext(r.Context())
	// Clone template to inject per-request T without racing shared funcMap.
	cloned, err := tmpl.Clone()
	if err != nil {
		slog.Error("clone template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned.Funcs(template.FuncMap{
		"T": func(key string) string { return i18n.T(locale, key) },
	})
	if err := cloned.ExecuteTemplate(w, "base.html", merged); err != nil {
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
	h.renderAppError(w, r, &appError{status: status, message: http.StatusText(status)})
}
