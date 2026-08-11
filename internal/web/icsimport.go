package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/datey/datey/internal/ics"
)

// icsPreviewItem is a single parsed event awaiting user confirmation.
type icsPreviewItem struct {
	Summary     string `json:"summary"`
	Date        string `json:"date"` // YYYY-MM-DD
	Type        string `json:"type"`
	PersonID    int    `json:"person_id"`
	PersonName  string `json:"person_name"`
	IsDuplicate bool   `json:"is_duplicate"`
	RecurYearly bool   `json:"recur_yearly"`
}

type icsImportPreviewData struct {
	Items    []icsPreviewItem
	Ready    int
	Dupe     int
	People   []personOption
	Types    []string
	Selected string
}

type icsImportResults struct {
	Imported int
	Skipped  int
}

// handleImportICS parses an uploaded .ics file and renders a preview of the
// events that would be created, flagging duplicates against existing events.
func (h *Handler) handleImportICS(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		slog.Error("import ics: parse multipart form", "error", err)
		h.renderICSMessage(w, r, "error", "File too large or invalid form.")
		return
	}

	file, _, err := r.FormFile("ics_file")
	if err != nil {
		slog.Error("import ics: get uploaded file", "error", err)
		h.renderICSMessage(w, r, "error", "No file uploaded.")
		return
	}
	defer func() { _ = file.Close() }()

	events, err := ics.Parse(file)
	if err != nil {
		slog.Error("import ics: parse", "error", err)
		h.renderICSMessage(w, r, "error", "Invalid iCalendar file: "+err.Error())
		return
	}

	personID, err := strconv.Atoi(r.FormValue("person_id"))
	if err != nil || personID <= 0 {
		h.renderICSMessage(w, r, "error", "A target person must be selected.")
		return
	}

	person, err := h.people.Get(r.Context(), personID)
	if err != nil {
		slog.Error("import ics: get person", "person_id", personID, "error", err)
		h.renderICSMessage(w, r, "error", "Target person not found.")
		return
	}

	eventType := r.FormValue("type")
	if eventType == "" {
		eventType = "custom"
	}

	// Build a set of existing events for duplicate detection.
	existing := make(map[string]bool)
	personEvents, err := h.events.ListByPerson(r.Context(), personID)
	if err == nil {
		for _, e := range personEvents {
			existing[e.Type+"|"+e.Date.Format("2006-01-02")] = true
		}
	} else {
		slog.Error("import ics: list person events", "person_id", personID, "error", err)
	}

	items := make([]icsPreviewItem, 0, len(events))
	for _, ev := range events {
		dateStr := ev.Start.Format("2006-01-02")
		items = append(items, icsPreviewItem{
			Summary:     ev.Summary,
			Date:        dateStr,
			Type:        eventType,
			PersonID:    personID,
			PersonName:  person.Name,
			IsDuplicate: existing[eventType+"|"+dateStr],
			RecurYearly: ev.RecurYearly,
		})
	}

	if len(items) == 0 {
		h.renderICSMessage(w, r, "error", "No importable events found in the uploaded file.")
		return
	}

	var ready, dupes int
	for _, it := range items {
		if it.IsDuplicate {
			dupes++
		} else {
			ready++
		}
	}

	toastMsg := fmt.Sprintf("%d event(s) ready. %d duplicate(s) flagged.", ready, dupes)
	toastType := "success"
	if ready == 0 {
		toastType = "error"
	}

	tmpl := h.templates["calendar.html"]
	if partial := tmpl.Lookup("icsImportPreview"); partial != nil {
		h.setHXTrigger(w, toastMsg, toastType)
		data := icsImportPreviewData{
			Items:    items,
			Ready:    ready,
			Dupe:     dupes,
			People:   h.personOptions(r.Context()),
			Types:    []string{"custom", "birthday", "anniversary", "wedding", "holiday", "meeting"},
			Selected: eventType,
		}
		if err := partial.Execute(w, data); err != nil {
			slog.Error("import ics: render preview", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	slog.Error("import ics: icsImportPreview template not found")
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// handleConfirmICS creates events from a previously previewed import payload.
// Duplicates are skipped unless the user opted to include them.
func (h *Handler) handleConfirmICS(w http.ResponseWriter, r *http.Request) {
	var items []icsPreviewItem
	payload := r.FormValue("payload")
	if payload == "" {
		h.renderICSMessage(w, r, "error", "No import data provided.")
		return
	}
	if err := json.Unmarshal([]byte(payload), &items); err != nil {
		slog.Error("import ics: decode payload", "error", err)
		h.renderICSMessage(w, r, "error", "Invalid import payload.")
		return
	}

	var imported, skipped int
	for _, it := range items {
		if it.IsDuplicate {
			skipped++
			continue
		}

		date, err := time.Parse("2006-01-02", it.Date)
		if err != nil {
			slog.Error("import ics: parse date", "date", it.Date, "error", err)
			skipped++
			continue
		}

		desc := it.Summary
		if desc == "" {
			desc = "Imported from iCal"
		}
		if _, err := h.events.CreateForPerson(r.Context(), it.PersonID, it.Type, date, desc); err != nil {
			slog.Error("import ics: create event", "person_id", it.PersonID, "error", err)
			skipped++
			continue
		}
		imported++
	}

	toastMsg := fmt.Sprintf("Imported %d event(s). %d skipped.", imported, skipped)
	toastType := "success"
	if imported == 0 {
		toastType = "error"
	}

	tmpl := h.templates["calendar.html"]
	if partial := tmpl.Lookup("icsImportResults"); partial != nil {
		h.setHXTrigger(w, toastMsg, toastType)
		if err := partial.Execute(w, icsImportResults{Imported: imported, Skipped: skipped}); err != nil {
			slog.Error("import ics: render results", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	slog.Error("import ics: icsImportResults template not found")
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// setHXTrigger writes the HX-Trigger header for a toast notification.
func (h *Handler) setHXTrigger(w http.ResponseWriter, message, toastType string) {
	payload := map[string]any{
		"show-toast": map[string]string{
			"message": message,
			"type":    toastType,
		},
	}
	b, _ := json.Marshal(payload)
	w.Header().Set("HX-Trigger", string(b))
}

// renderICSMessage shows a toast and an empty preview area for error cases.
func (h *Handler) renderICSMessage(w http.ResponseWriter, r *http.Request, toastType, message string) {
	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, "/calendar?error="+message, http.StatusSeeOther)
		return
	}
	h.setHXTrigger(w, message, toastType)
	tmpl := h.templates["calendar.html"]
	if partial := tmpl.Lookup("icsImportPreview"); partial != nil {
		if err := partial.Execute(w, icsImportPreviewData{}); err != nil {
			slog.Error("import ics: render error preview", "error", err)
		}
		return
	}
	http.Error(w, message, http.StatusBadRequest)
}
