package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/milestone"
)

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func writeAPIJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeAPIJSON(w, status, map[string]string{"error": code, "message": message})
}

// DTOs

type UserDTO struct {
	ID             int    `json:"id"`
	Username       string `json:"username"`
	Role           string `json:"role"`
	LinkedPersonID *int   `json:"linked_person_id"`
}

type UpcomingEventDTO struct {
	ID             int     `json:"id"`
	PersonID       *int    `json:"person_id"`
	PersonName     string  `json:"person_name"`
	PersonPhotoURL *string `json:"person_photo_url"`
	GroupID        *int    `json:"group_id"`
	GroupName      *string `json:"group_name"`
	Type           string  `json:"type"`
	Description    string  `json:"description"`
	Date           string  `json:"date"`
	DaysRemaining  int     `json:"days_remaining"`
	TurningAge     *int    `json:"turning_age"`
	YearsTogether  *int    `json:"years_together"`
	CalendarSystem string  `json:"calendar_system"`
	LunarLabel     *string `json:"lunar_label"`
	IsLeapMonth    bool    `json:"is_leap_month"`
	IsMilestone    bool    `json:"is_milestone"`
	MilestoneLabel *string `json:"milestone_label"`
}

type PersonCardDTO struct {
	ID        int           `json:"id"`
	Name      string        `json:"name"`
	FirstName string        `json:"first_name"`
	LastName  string        `json:"last_name"`
	PhotoURL  *string       `json:"photo_url"`
	Tags      []string      `json:"tags"`
	Groups    []GroupRefDTO `json:"groups"`
	NextEvent *NextEventDTO `json:"next_event"`
}

type GroupRefDTO struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type NextEventDTO struct {
	Type          string `json:"type"`
	Date          string `json:"date"`
	DaysRemaining int    `json:"days_remaining"`
}

type GiftIdeaDTO struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	Notes      string `json:"notes"`
	PriceCents *int   `json:"price_cents"`
	URL        string `json:"url"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type EventDTO struct {
	ID             int    `json:"id"`
	Type           string `json:"type"`
	Date           string `json:"date"`
	Description    string `json:"description"`
	CalendarSystem string `json:"calendar_system"`
	LunarMonth     *int   `json:"lunar_month"`
	LunarDay       *int   `json:"lunar_day"`
	LunarLeap      bool   `json:"lunar_leap"`
}

type RelationshipDTO struct {
	ID              int     `json:"id"`
	Type            string  `json:"type"`
	Label           *string `json:"label"`
	OtherPersonID   int     `json:"other_person_id"`
	OtherPersonName string  `json:"other_person_name"`
	Direction       string  `json:"direction"`
}

func (h *Handler) apiMe(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
		return
	}
	writeAPIJSON(w, http.StatusOK, UserDTO{ID: u.ID, Username: u.Username, Role: string(u.Role), LinkedPersonID: u.LinkedPersonID})
}

func (h *Handler) oidcStatus(w http.ResponseWriter, r *http.Request) {
	writeAPIJSON(w, http.StatusOK, map[string]bool{"enabled": h.oidcEnabled()})
}

func (h *Handler) setLinkedPerson(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
		return
	}
	var body struct {
		PersonID *int `json:"person_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid JSON")
		return
	}
	if body.PersonID != nil {
		if _, err := h.people.Get(r.Context(), *body.PersonID); err != nil {
			writeAPIError(w, http.StatusNotFound, "not_found", "Person not found")
			return
		}
	}
	updated, err := h.users.SetLinkedPerson(r.Context(), u.ID, body.PersonID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to update")
		return
	}
	writeAPIJSON(w, http.StatusOK, UserDTO{ID: updated.ID, Username: updated.Username, Role: string(updated.Role), LinkedPersonID: updated.LinkedPersonID})
}

func (h *Handler) dashboardJSON(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	reminderDays := h.cfg.ReminderDays
	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if days, err := strconv.Atoi(daysStr); err == nil && days >= 1 && days <= 365 {
			reminderDays = days
		}
	}
	end := now.AddDate(0, 0, reminderDays)
	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), todayStartUTC(), end)
	if err != nil {
		slog.Error("dashboard json: list upcoming", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal", "failed")
		return
	}
	hasBirthday := false
	for _, occ := range occurrences {
		if occ.Event.Type == "birthday" {
			hasBirthday = true
			break
		}
	}
	if !hasBirthday {
		if nb, err := h.events.NextBirthdayOccurrence(r.Context(), now); err == nil && nb != nil {
			occurrences = append(occurrences, *nb)
		}
	}

	var todayEvents, weekEvents, monthEvents, laterEvents []UpcomingEventDTO
	for _, occ := range occurrences {
		e := occ.Event
		occDate := occ.Date
		days := calendarDayDelta(dateOnly(now, now.Location()), dateOnly(occDate, time.UTC))
		if days < 0 {
			days = 0
		}
		dto := UpcomingEventDTO{
			ID:             e.ID,
			Type:           e.Type,
			Description:    e.Description,
			Date:           occDate.Format("2006-01-02"),
			DaysRemaining:  days,
			CalendarSystem: e.CalendarSystem,
			IsLeapMonth:    e.LunarLeap,
		}
		if dto.CalendarSystem == "" {
			dto.CalendarSystem = "gregorian"
		}
		if e.Edges.Person != nil {
			pid := e.Edges.Person.ID
			dto.PersonID = &pid
			dto.PersonName = e.Edges.Person.Name
			if url := h.photoURL(r, e.Edges.Person); url != "" {
				dto.PersonPhotoURL = &url
			}
		} else if e.Edges.Group != nil {
			gid := e.Edges.Group.ID
			gname := e.Edges.Group.Name
			dto.GroupID = &gid
			dto.GroupName = &gname
		} else {
			dto.PersonName = eventOwnerName(e)
		}
		if e.Type == "birthday" && e.Date.Year() > 1 {
			if e.CalendarSystem == "lunar" && e.LunarMonth != nil && e.LunarDay != nil {
				years := now.Year() - e.Date.Year()
				disp, _ := displayDateForEvent(e, now)
				if dateOnly(disp, time.UTC).After(dateOnly(now, now.Location())) {
					years--
				}
				if years >= 0 {
					ta := years + 1
					dto.TurningAge = &ta
				}
			} else {
				info := age.InfoFor(e.Date, now)
				if info.HasAge {
					ta := info.Next
					dto.TurningAge = &ta
				}
			}
		}
		if e.Type == "anniversary" && e.Date.Year() > 1 {
			yt := occDate.Year() - e.Date.Year()
			if yt >= 0 {
				dto.YearsTogether = &yt
			}
		}
		if e.CalendarSystem == "lunar" && e.LunarMonth != nil && e.LunarDay != nil {
			lbl := formatLunarNotation(e)
			dto.LunarLabel = &lbl
		}
		if ok, label := milestone.IsMilestone(e.Type, e.Date, occDate); ok {
			dto.IsMilestone = true
			dto.MilestoneLabel = &label
		}
		switch {
		case days <= 1:
			todayEvents = append(todayEvents, dto)
		case days <= 7:
			weekEvents = append(weekEvents, dto)
		case days <= 30:
			monthEvents = append(monthEvents, dto)
		default:
			laterEvents = append(laterEvents, dto)
		}
	}
	if todayEvents == nil {
		todayEvents = []UpcomingEventDTO{}
	}
	if weekEvents == nil {
		weekEvents = []UpcomingEventDTO{}
	}
	if monthEvents == nil {
		monthEvents = []UpcomingEventDTO{}
	}
	if laterEvents == nil {
		laterEvents = []UpcomingEventDTO{}
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{
		"today":      todayEvents,
		"this_week":  weekEvents,
		"this_month": monthEvents,
		"later":      laterEvents,
	})
}
