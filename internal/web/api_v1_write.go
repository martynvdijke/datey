package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/datey/datey/ent"
	personname "github.com/datey/datey/internal/person"
	"github.com/datey/datey/internal/recurring"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/session"
)

// Versioned JSON write API (phase 1: people and events).
//
// Authentication is bearer API token OR browser session, matching /api/v1/me.
// APIAuthJSON resolves a session cookie itself so unauthenticated calls get a
// JSON 401 instead of the login redirect the browser-oriented Auth middleware
// issues. The shared CSRF middleware still runs for cookie-authenticated
// POST/DELETE and skips bearer requests entirely.
//
// Error shape:
//
//	{"error": "<code>", "message": "<safe text>", "fields": {"<field>": "<code>"}}
//
// `fields` appears for 400 validation failures only. Field codes are
// machine-readable (never localized) so clients can translate them.

// writeAPIValidationError reports per-field validation failures.
func writeAPIValidationError(w http.ResponseWriter, fields map[string]string) {
	writeAPIJSON(w, http.StatusBadRequest, map[string]any{
		"error":   "validation",
		"message": "Validation failed",
		"fields":  fields,
	})
}

// APIAuthJSON authenticates a request via bearer token (already resolved by
// BearerAuth) or a session cookie, and answers 401 in JSON when neither is
// present. It mirrors Auth without the HTML redirect.
func (h *Handler) APIAuthJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFromContext(r.Context()) != nil {
			next.ServeHTTP(w, r)
			return
		}
		token, err := session.ReadCookie(r)
		if err != nil {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
			return
		}
		sess, err := h.sessions.GetByToken(r.Context(), token)
		if err != nil {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
			return
		}
		u, err := h.users.GetByID(r.Context(), sess.Edges.User.ID)
		if err != nil {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, u)
		ctx = context.WithValue(ctx, "user", u) //nolint:staticcheck
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// decodeJSONBody decodes a bounded JSON request body, answering 400 when the
// payload is malformed. Unknown fields are ignored for forward compatibility.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return false
	}
	return true
}

// ── People ───────────────────────────────────────────────────────────────

// PersonWriteDTO is the people payload returned by the write API.
type PersonWriteDTO struct {
	ID              int           `json:"id"`
	Name            string        `json:"name"`
	FirstName       string        `json:"first_name"`
	MiddleName      string        `json:"middle_name"`
	LastName        string        `json:"last_name"`
	Notes           string        `json:"notes"`
	NotifyBirthdays bool          `json:"notify_birthdays"`
	Groups          []GroupRefDTO `json:"groups"`
	Tags            []string      `json:"tags"`
}

type personWriteRequest struct {
	Name            *string   `json:"name"`
	FirstName       *string   `json:"first_name"`
	MiddleName      *string   `json:"middle_name"`
	LastName        *string   `json:"last_name"`
	Notes           *string   `json:"notes"`
	Groups          *[]int    `json:"groups"`
	Tags            *[]string `json:"tags"`
	NotifyBirthdays *bool     `json:"notify_birthdays"`
}

// nameParts returns the trimmed structured name parts plus the legacy display
// name fallback. Nil parts stay empty so callers can tell "absent" from
// "cleared" by inspecting the request fields themselves.
func (req personWriteRequest) nameParts() (first, middle, last, legacy string) {
	if req.FirstName != nil {
		first = strings.TrimSpace(*req.FirstName)
	}
	if req.MiddleName != nil {
		middle = strings.TrimSpace(*req.MiddleName)
	}
	if req.LastName != nil {
		last = strings.TrimSpace(*req.LastName)
	}
	if req.Name != nil {
		legacy = strings.TrimSpace(*req.Name)
	}
	return first, middle, last, legacy
}

// existingNameParts reads the stored structured parts, treating NULL as empty.
func existingNameParts(p *ent.Person) (first, middle, last string) {
	if p.FirstName != nil {
		first = *p.FirstName
	}
	if p.MiddleName != nil {
		middle = *p.MiddleName
	}
	if p.LastName != nil {
		last = *p.LastName
	}
	return first, middle, last
}

// personWriteDTO builds the response payload, loading the group and tag edges
// that the repository accessors do not preload after a mutation.
func (h *Handler) personWriteDTO(ctx context.Context, p *ent.Person) PersonWriteDTO {
	dto := PersonWriteDTO{
		ID:              p.ID,
		Name:            p.Name,
		Notes:           p.Notes,
		NotifyBirthdays: p.NotifyBirthdays,
		Groups:          []GroupRefDTO{},
		Tags:            []string{},
	}
	dto.FirstName, dto.MiddleName, dto.LastName = existingNameParts(p)
	if groups, err := h.groups.ListByPerson(ctx, p.ID); err == nil {
		for _, g := range groups {
			dto.Groups = append(dto.Groups, GroupRefDTO{ID: g.ID, Name: g.Name})
		}
	}
	if tags, err := h.tags.ListByPerson(ctx, p.ID); err == nil {
		for _, t := range tags {
			dto.Tags = append(dto.Tags, t.Name)
		}
	}
	return dto
}

// validateGroupIDs checks that every referenced group exists. It returns a
// machine-readable code, or "" when all IDs are valid.
func (h *Handler) validateGroupIDs(ctx context.Context, ids []int) string {
	for _, id := range ids {
		if id <= 0 {
			return "invalid_group"
		}
		if _, err := h.groups.GetByID(ctx, id); err != nil {
			return "unknown_group"
		}
	}
	return ""
}

// validateTagNames normalizes and validates tag names, returning a code when
// any name breaks the tag grammar.
func validateTagNames(names []string) (normalized []string, code string) {
	seen := make(map[string]bool, len(names))
	for _, raw := range names {
		n := repository.NormalizeTag(raw)
		if n == "" || !repository.ValidateTagName(n) {
			return nil, "invalid_tag"
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		normalized = append(normalized, n)
	}
	return normalized, ""
}

// syncPersonTags replaces a person's tags with exactly the given names.
func (h *Handler) syncPersonTags(ctx context.Context, personID int, names []string) error {
	desired := make(map[string]bool, len(names))
	for _, n := range names {
		desired[n] = true
	}
	if current, err := h.tags.ListByPerson(ctx, personID); err == nil {
		for _, t := range current {
			if !desired[t.Name] {
				_ = h.tags.RemoveFromPerson(ctx, personID, t.Name)
			}
		}
	}
	for _, n := range names {
		if err := h.tags.AddToPerson(ctx, personID, n); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) apiCreatePerson(w http.ResponseWriter, r *http.Request) {
	var req personWriteRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	first, middle, last, legacy := req.nameParts()
	display := personname.DisplayName(first, middle, last, legacy)
	notes := ""
	if req.Notes != nil {
		notes = *req.Notes
	}

	fields := map[string]string{}
	if display == "" {
		fields["name"] = "required"
	}
	var tagNames []string
	if req.Groups != nil {
		if code := h.validateGroupIDs(r.Context(), *req.Groups); code != "" {
			fields["groups"] = code
		}
	}
	if req.Tags != nil {
		var code string
		tagNames, code = validateTagNames(*req.Tags)
		if code != "" {
			fields["tags"] = code
		}
	}
	if len(fields) > 0 {
		writeAPIValidationError(w, fields)
		return
	}

	// Person.name is globally unique; answer 409 rather than surfacing the
	// constraint error. The pre-check gives a clean message, the constraint
	// check below closes the race.
	if existing, err := h.people.FindByName(r.Context(), display); err == nil && existing != nil {
		writeAPIError(w, http.StatusConflict, "conflict", "A person with this name already exists")
		return
	}

	p, err := h.people.CreateStructured(r.Context(), display, first, middle, last, notes, "")
	if err != nil {
		if ent.IsConstraintError(err) {
			writeAPIError(w, http.StatusConflict, "conflict", "A person with this name already exists")
			return
		}
		slog.Error("api: create person", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to create person")
		return
	}

	if req.Groups != nil {
		if err := h.syncPersonGroups(r.Context(), p.ID, *req.Groups); err != nil {
			slog.Error("api: create person groups", "person_id", p.ID, "error", err)
		}
	}
	if len(tagNames) > 0 {
		if err := h.syncPersonTags(r.Context(), p.ID, tagNames); err != nil {
			slog.Error("api: create person tags", "person_id", p.ID, "error", err)
		}
	}
	if req.NotifyBirthdays != nil && *req.NotifyBirthdays != p.NotifyBirthdays {
		if updated, err := h.people.SetNotifyBirthdays(r.Context(), p.ID, *req.NotifyBirthdays); err == nil {
			p = updated
		} else {
			slog.Error("api: set notify_birthdays on create", "person_id", p.ID, "error", err)
		}
	}

	h.auditRecord(r, "person.create", strconv.Itoa(p.ID))
	writeAPIJSON(w, http.StatusCreated, h.personWriteDTO(r.Context(), p))
}

func (h *Handler) apiUpdatePerson(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid person id")
		return
	}
	person, err := h.people.Get(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Person not found")
			return
		}
		slog.Error("api: get person", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to load person")
		return
	}

	var req personWriteRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	first, middle, last := existingNameParts(person)
	display := person.Name
	legacy := person.Name
	structuredChanged := req.FirstName != nil || req.MiddleName != nil || req.LastName != nil
	nameChanged := req.Name != nil
	if req.FirstName != nil {
		first = strings.TrimSpace(*req.FirstName)
	}
	if req.MiddleName != nil {
		middle = strings.TrimSpace(*req.MiddleName)
	}
	if req.LastName != nil {
		last = strings.TrimSpace(*req.LastName)
	}
	if nameChanged {
		legacy = strings.TrimSpace(*req.Name)
	}
	switch {
	case structuredChanged:
		display = personname.DisplayName(first, middle, last, legacy)
	case nameChanged:
		display = legacy
	}

	notes := person.Notes
	if req.Notes != nil {
		notes = *req.Notes
	}

	fields := map[string]string{}
	if display == "" {
		fields["name"] = "required"
	}
	var tagNames []string
	if req.Groups != nil {
		if code := h.validateGroupIDs(r.Context(), *req.Groups); code != "" {
			fields["groups"] = code
		}
	}
	if req.Tags != nil {
		var code string
		tagNames, code = validateTagNames(*req.Tags)
		if code != "" {
			fields["tags"] = code
		}
	}
	if len(fields) > 0 {
		writeAPIValidationError(w, fields)
		return
	}

	updated, err := h.people.UpdateStructured(r.Context(), id, display, first, middle, last, notes, "")
	if err != nil {
		if ent.IsConstraintError(err) {
			writeAPIError(w, http.StatusConflict, "conflict", "A person with this name already exists")
			return
		}
		if ent.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Person not found")
			return
		}
		slog.Error("api: update person", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to update person")
		return
	}

	if req.Groups != nil {
		if err := h.syncPersonGroups(r.Context(), id, *req.Groups); err != nil {
			slog.Error("api: update person groups", "person_id", id, "error", err)
			writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to update groups")
			return
		}
	}
	if req.Tags != nil {
		if err := h.syncPersonTags(r.Context(), id, tagNames); err != nil {
			slog.Error("api: update person tags", "person_id", id, "error", err)
			writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to update tags")
			return
		}
	}
	if req.NotifyBirthdays != nil {
		updated, err = h.people.SetNotifyBirthdays(r.Context(), id, *req.NotifyBirthdays)
		if err != nil {
			slog.Error("api: update notify_birthdays", "person_id", id, "error", err)
			writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to update person")
			return
		}
	}

	h.auditRecord(r, "person.update", strconv.Itoa(id))
	writeAPIJSON(w, http.StatusOK, h.personWriteDTO(r.Context(), updated))
}

func (h *Handler) apiDeletePerson(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid person id")
		return
	}
	if _, err := h.people.Get(r.Context(), id); err != nil {
		if ent.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Person not found")
			return
		}
		slog.Error("api: get person for delete", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to delete person")
		return
	}

	// Same cascade as the web delete path (gift ideas + relationships first).
	if err := h.people.Delete(r.Context(), id); err != nil {
		if ent.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Person not found")
			return
		}
		slog.Error("api: delete person", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to delete person")
		return
	}
	if h.photoStore != nil {
		if err := h.photoStore.DeletePerson(id); err != nil {
			slog.Warn("api: delete person photo files", "person_id", id, "error", err)
		}
	}

	h.auditRecord(r, "person.delete", strconv.Itoa(id))
	w.WriteHeader(http.StatusNoContent)
}

// ── Events ───────────────────────────────────────────────────────────────

// EventWriteDTO is the event payload returned by the write API. It carries the
// schedule and calendar fields the HTML views have always stored.
type EventWriteDTO struct {
	ID              int     `json:"id"`
	PersonID        *int    `json:"person_id"`
	Type            string  `json:"type"`
	Date            string  `json:"date"`
	Description     string  `json:"description"`
	Notes           string  `json:"notes"`
	CalendarSystem  string  `json:"calendar_system"`
	LunarMonth      *int    `json:"lunar_month"`
	LunarDay        *int    `json:"lunar_day"`
	LunarLeap       bool    `json:"lunar_leap"`
	Frequency       string  `json:"frequency"`
	Interval        int     `json:"interval"`
	EndDate         *string `json:"end_date"`
	OccurrenceCount *int    `json:"occurrence_count"`
}

type eventWriteRequest struct {
	Type            *string `json:"type"`
	Date            *string `json:"date"`
	Description     *string `json:"description"`
	Notes           *string `json:"notes"`
	CalendarSystem  *string `json:"calendar_system"`
	LunarMonth      *int    `json:"lunar_month"`
	LunarDay        *int    `json:"lunar_day"`
	LunarLeap       *bool   `json:"lunar_leap"`
	Frequency       *string `json:"frequency"`
	Interval        *int    `json:"interval"`
	EndDate         *string `json:"end_date"`
	OccurrenceCount *int    `json:"occurrence_count"`
}

// intString renders an optional int as the string form the shared validator
// expects ("" when absent).
func intString(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

// eventInputFromStored maps a stored event back into the raw input shape so a
// PATCH can validate the merged result with the same rules a create uses.
func eventInputFromStored(e *ent.Event) eventFormInput {
	endDate := ""
	if e.EndDate != nil {
		endDate = e.EndDate.Format("2006-01-02")
	}
	return eventFormInput{
		Type:            e.Type,
		Date:            e.Date.Format("2006-01-02"),
		Description:     e.Description,
		Notes:           e.Notes,
		CalendarSystem:  e.CalendarSystem,
		LunarMonth:      intString(e.LunarMonth),
		LunarDay:        intString(e.LunarDay),
		LunarLeap:       e.LunarLeap,
		Frequency:       e.Frequency,
		Interval:        strconv.Itoa(e.Interval),
		EndDate:         endDate,
		OccurrenceCount: intString(e.OccurrenceCount),
	}
}

// apply overlays the fields present in the request onto base. An explicit
// empty end_date clears the bound; occurrence_count 0 clears it as well.
func (req eventWriteRequest) apply(base eventFormInput) eventFormInput {
	if req.Type != nil {
		base.Type = *req.Type
	}
	if req.Date != nil {
		base.Date = *req.Date
	}
	if req.Description != nil {
		base.Description = *req.Description
	}
	if req.Notes != nil {
		base.Notes = *req.Notes
	}
	if req.CalendarSystem != nil {
		base.CalendarSystem = *req.CalendarSystem
	}
	if req.LunarMonth != nil {
		base.LunarMonth = strconv.Itoa(*req.LunarMonth)
	}
	if req.LunarDay != nil {
		base.LunarDay = strconv.Itoa(*req.LunarDay)
	}
	if req.LunarLeap != nil {
		base.LunarLeap = *req.LunarLeap
	}
	if req.Frequency != nil {
		base.Frequency = *req.Frequency
	}
	if req.Interval != nil {
		base.Interval = strconv.Itoa(*req.Interval)
	}
	if req.EndDate != nil {
		base.EndDate = *req.EndDate
	}
	if req.OccurrenceCount != nil {
		if *req.OccurrenceCount == 0 {
			base.OccurrenceCount = ""
		} else {
			base.OccurrenceCount = strconv.Itoa(*req.OccurrenceCount)
		}
	}
	return base
}

// eventWriteDTO renders an event for the JSON API, defaulting the columns that
// predate the calendar/schedule work.
func eventWriteDTO(e *ent.Event) EventWriteDTO {
	dto := EventWriteDTO{
		ID:              e.ID,
		Type:            e.Type,
		Date:            e.Date.Format("2006-01-02"),
		Description:     e.Description,
		Notes:           e.Notes,
		CalendarSystem:  e.CalendarSystem,
		LunarMonth:      e.LunarMonth,
		LunarDay:        e.LunarDay,
		LunarLeap:       e.LunarLeap,
		Frequency:       e.Frequency,
		Interval:        e.Interval,
		OccurrenceCount: e.OccurrenceCount,
	}
	if dto.CalendarSystem == "" {
		dto.CalendarSystem = "gregorian"
	}
	if dto.Frequency == "" {
		dto.Frequency = recurring.FrequencyAnnual
	}
	if e.EndDate != nil {
		end := e.EndDate.Format("2006-01-02")
		dto.EndDate = &end
	}
	if e.Edges.Person != nil {
		pid := e.Edges.Person.ID
		dto.PersonID = &pid
	}
	return dto
}

func (h *Handler) apiListPersonEvents(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid person id")
		return
	}
	if _, err := h.people.Get(r.Context(), id); err != nil {
		if ent.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Person not found")
			return
		}
		slog.Error("api: get person for events", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to load person")
		return
	}

	events, err := h.events.ListByPerson(r.Context(), id)
	if err != nil {
		slog.Error("api: list person events", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to load events")
		return
	}
	out := make([]EventWriteDTO, 0, len(events))
	for _, e := range events {
		out = append(out, eventWriteDTO(e))
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"person_id": id, "events": out})
}

func (h *Handler) apiCreateEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid person id")
		return
	}
	if _, err := h.people.Get(r.Context(), id); err != nil {
		if ent.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Person not found")
			return
		}
		slog.Error("api: get person for event create", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to load person")
		return
	}

	var req eventWriteRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	in := req.apply(eventFormInput{})
	validated, codes := validateEventInput(in)
	if len(codes) > 0 {
		writeAPIValidationError(w, codes)
		return
	}

	schedule := repository.NewEventSchedule(validated.Frequency, validated.Interval, validated.EndDate, validated.OccurrenceCount)
	created, err := h.events.CreateForPersonWithSchedule(r.Context(), id, validated.Type, validated.Date, in.Description, validated.CalendarSystem, validated.StoredMonth, validated.LunarDay, validated.StoredLeap, schedule)
	if err != nil {
		slog.Error("api: create event", "person_id", id, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to create event")
		return
	}
	if in.Notes != "" {
		if created, err = h.events.SetNotes(r.Context(), created.ID, in.Notes); err != nil {
			slog.Error("api: set event notes", "event_id", created.ID, "error", err)
			writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to create event")
			return
		}
	}
	// Reload with the owner edge so the response carries person_id.
	if withOwner, err := h.events.GetWithOwner(r.Context(), created.ID); err == nil {
		created = withOwner
	}

	h.auditRecord(r, "event.create", strconv.Itoa(created.ID))
	writeAPIJSON(w, http.StatusCreated, eventWriteDTO(created))
}

func (h *Handler) apiUpdateEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid event id")
		return
	}
	existing, err := h.events.Get(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Event not found")
			return
		}
		slog.Error("api: get event", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to load event")
		return
	}

	var req eventWriteRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	in := req.apply(eventInputFromStored(existing))
	validated, codes := validateEventInput(in)
	if len(codes) > 0 {
		writeAPIValidationError(w, codes)
		return
	}

	schedule := repository.NewEventSchedule(validated.Frequency, validated.Interval, validated.EndDate, validated.OccurrenceCount)
	updated, err := h.events.UpdateWithSchedule(r.Context(), id, validated.Type, validated.Date, in.Description, validated.CalendarSystem, validated.StoredMonth, validated.LunarDay, validated.StoredLeap, schedule)
	if err != nil {
		if ent.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Event not found")
			return
		}
		slog.Error("api: update event", "event_id", id, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to update event")
		return
	}
	if in.Notes != existing.Notes {
		if updated, err = h.events.SetNotes(r.Context(), id, in.Notes); err != nil {
			slog.Error("api: set event notes", "event_id", id, "error", err)
			writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to update event")
			return
		}
	}
	// Reload with the owner edge so the response carries person_id.
	if withOwner, err := h.events.GetWithOwner(r.Context(), id); err == nil {
		updated = withOwner
	}

	h.auditRecord(r, "event.update", strconv.Itoa(id))
	writeAPIJSON(w, http.StatusOK, eventWriteDTO(updated))
}

func (h *Handler) apiDeleteEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid event id")
		return
	}
	if _, err := h.events.Get(r.Context(), id); err != nil {
		if ent.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Event not found")
			return
		}
		slog.Error("api: get event for delete", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to delete event")
		return
	}
	if err := h.events.Delete(r.Context(), id); err != nil {
		if ent.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Event not found")
			return
		}
		slog.Error("api: delete event", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to delete event")
		return
	}

	h.auditRecord(r, "event.delete", strconv.Itoa(id))
	w.WriteHeader(http.StatusNoContent)
}
