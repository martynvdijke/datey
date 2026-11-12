package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/datey/datey/internal/csvimport"
	"github.com/datey/datey/internal/i18n"
	"github.com/datey/datey/internal/vcard"
)

// csvImportMaxUploadSize caps an uploaded CSV. Birthdays lists are small; 5 MB
// is far above any realistic spreadsheet and keeps a bad upload harmless.
const csvImportMaxUploadSize = 5 << 20

// csvImportDefaults is a render-time fallback for the template's i18n keys so
// the page is readable before the keys land in internal/i18n/locales/en.json.
// The keys and English copy are reported to the maintainer for translation;
// i18n.T still wins as soon as a catalog entry exists.
var csvImportDefaults = map[string]string{
	"csv.import.title":              "Import people from CSV",
	"csv.import.help":               "Upload a spreadsheet exported as CSV. Each row becomes a person with a birthday event. Existing people are never changed.",
	"csv.import.file_label":         "CSV file",
	"csv.import.file_hint":          "Columns are detected from the header (Name, Birthday, Notes, Groups, Tags). Without a header the first column is the name and the second the birthday.",
	"csv.import.preview":            "Preview",
	"csv.import.back":               "Back to people",
	"csv.import.summary_total":      "Rows",
	"csv.import.summary_importable": "Importable",
	"csv.import.summary_skipped":    "Skipped",
	"csv.import.mapping":            "Detected columns",
	"csv.import.mapping_positional": "No header row found — columns are read positionally.",
	"csv.import.warnings":           "Warnings",
	"csv.import.unknown_groups":     "Groups that do not exist yet (created on import)",
	"csv.import.importable_heading": "Will be imported",
	"csv.import.skipped_heading":    "Skipped rows",
	"csv.import.confirm":            "Import people",
	"csv.import.confirm_hint":       "Nothing has been saved yet. Existing people and duplicate rows are skipped.",
	"csv.import.no_preview":         "Upload a CSV file to see what would be imported.",
	"csv.import.col_name":           "Name",
	"csv.import.col_birthday":       "Birthday",
	"csv.import.col_groups":         "Groups",
	"csv.import.col_tags":           "Tags",
	"csv.import.col_reason":         "Reason",
	"csv.import.results_imported":   "Imported",
	"csv.import.results_skipped":    "Skipped",
	"csv.import.results_birthdays":  "Birthday events created",
	"csv.import.results_groups":     "Group memberships added",
	"csv.import.results_tags":       "Tags added",
}

// csvImportView is the data for the full page and the preview partial.
type csvImportView struct {
	HasPreview     bool
	Total          int
	Importable     int
	Skipped        int
	ImportableRows []csvimport.Row
	SkippedRows    []csvimport.Row
	Mapping        []csvimport.FieldMap
	Positional     bool
	HeaderRow      bool
	Warnings       []string
	UnknownGroups  []string
	CSRFToken      string
}

// fields flattens the view for the page render, which merges it into the base
// template data (a map).
func (v csvImportView) fields() map[string]any {
	return map[string]any{
		"HasPreview":     v.HasPreview,
		"Total":          v.Total,
		"Importable":     v.Importable,
		"Skipped":        v.Skipped,
		"ImportableRows": v.ImportableRows,
		"SkippedRows":    v.SkippedRows,
		"Mapping":        v.Mapping,
		"Positional":     v.Positional,
		"HeaderRow":      v.HeaderRow,
		"Warnings":       v.Warnings,
		"UnknownGroups":  v.UnknownGroups,
		"CSRFToken":      v.CSRFToken,
	}
}

// csvImportResults is the summary shown after a confirmed import.
type csvImportResults struct {
	Imported  int
	Skipped   int
	Birthdays int
	Groups    int
	Tags      int
}

var (
	csvImportTemplateOnce sync.Once
	csvImportTemplate     *template.Template
	csvImportTemplateErr  error
)

// csvTemplate loads the CSV import page together with the base layout. The
// page is deliberately kept out of loadTemplates so the feature touches no
// shared template registry.
func csvTemplate() (*template.Template, error) {
	csvImportTemplateOnce.Do(func() {
		csvImportTemplate, csvImportTemplateErr = template.New("base.html").
			Funcs(funcMap).
			ParseFS(templateFS, "templates/base.html", "templates/csv_import.html")
	})
	return csvImportTemplate, csvImportTemplateErr
}

// csvTranslator returns a per-request T that falls back to the built-in
// English copy above when a catalog entry is missing.
func csvTranslator(locale string) func(string) string {
	return func(key string) string {
		if v := i18n.T(locale, key); v != key {
			return v
		}
		if v, ok := csvImportDefaults[key]; ok {
			return v
		}
		return key
	}
}

// renderCSVPage renders the full CSV import page.
func (h *Handler) renderCSVPage(w http.ResponseWriter, r *http.Request, view csvImportView) {
	tmpl, err := csvTemplate()
	if err != nil {
		slog.Error("csv import: load template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned, err := tmpl.Clone()
	if err != nil {
		slog.Error("csv import: clone template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned.Funcs(template.FuncMap{"T": csvTranslator(i18n.LocaleFromContext(r.Context()))})

	const title = "Datey - Import CSV"
	data := h.baseData(r, title)
	view.CSRFToken = csrfTokenFromContext(r.Context())
	maps.Copy(data, view.fields())
	data["Title"] = title

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := cloned.ExecuteTemplate(w, "base.html", data); err != nil {
		slog.Error("csv import: render page", "error", err)
	}
}

// renderCSVPartial renders one named partial from csv_import.html. HTMX
// requests get the fragment; plain requests get the whole page so a form
// submitted without JavaScript still works.
func (h *Handler) renderCSVPartial(w http.ResponseWriter, r *http.Request, name string, view csvImportView, data any) {
	if r.Header.Get("HX-Request") != "true" {
		h.renderCSVPage(w, r, view)
		return
	}
	tmpl, err := csvTemplate()
	if err != nil {
		slog.Error("csv import: load template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned, err := tmpl.Clone()
	if err != nil {
		slog.Error("csv import: clone template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned.Funcs(template.FuncMap{"T": csvTranslator(i18n.LocaleFromContext(r.Context()))})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := cloned.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("csv import: render partial", "name", name, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// csvImportMessage reports a problem without clearing the page. HTMX clients
// get a toast plus an empty preview; plain clients get a redirect with the
// message in the query string, mirroring the other import handlers.
func (h *Handler) csvImportMessage(w http.ResponseWriter, r *http.Request, message string) {
	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, "/people/import/csv?error="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	toastHeader(w, message, "error")
	empty := csvImportView{CSRFToken: csrfTokenFromContext(r.Context())}
	h.renderCSVPartial(w, r, "csvImportPreviewBody", empty, empty)
}

// handleCSVImportForm renders the empty upload page.
func (h *Handler) handleCSVImportForm(w http.ResponseWriter, r *http.Request) {
	h.renderCSVPage(w, r, csvImportView{
		CSRFToken: csrfTokenFromContext(r.Context()),
	})
}

// handleCSVImportPreview parses an uploaded CSV and renders the preview: what
// would be imported, what is skipped and why, and any file-level warnings.
// Nothing is written here.
func (h *Handler) handleCSVImportPreview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, csvImportMaxUploadSize)

	if err := r.ParseMultipartForm(csvImportMaxUploadSize); err != nil {
		slog.Error("csv import: parse multipart form", "error", err)
		h.csvImportMessage(w, r, "File too large or invalid form. The limit is 5 MB.")
		return
	}

	file, header, err := r.FormFile("csv_file")
	if err != nil {
		slog.Error("csv import: get uploaded file", "error", err)
		h.csvImportMessage(w, r, "No file uploaded.")
		return
	}
	defer func() { _ = file.Close() }()

	if header.Size > csvImportMaxUploadSize {
		h.csvImportMessage(w, r, "File too large. The limit is 5 MB.")
		return
	}
	if !csvFileAllowed(header.Filename, header.Header.Get("Content-Type")) {
		h.csvImportMessage(w, r, "Only .csv or .txt files are supported.")
		return
	}

	result, err := csvimport.Parse(file)
	if err != nil {
		slog.Error("csv import: parse csv", "file", header.Filename, "error", err)
		h.csvImportMessage(w, r, "Invalid CSV file: "+err.Error())
		return
	}
	if result.Total() == 0 {
		h.csvImportMessage(w, r, "No rows found in the uploaded file.")
		return
	}

	// Flag people that already exist and groups that do not exist yet, so the
	// preview tells the whole story before anything is written.
	result.MarkExisting(h.existingPersonNames(r))
	unknownGroups := result.UnknownGroups(h.existingGroupNames(r))

	view := csvImportView{
		HasPreview:    true,
		Total:         result.Total(),
		Mapping:       result.Mapping.Fields,
		Positional:    result.Mapping.Positional,
		HeaderRow:     result.Mapping.HeaderRow,
		Warnings:      result.Warnings,
		UnknownGroups: unknownGroups,
		CSRFToken:     csrfTokenFromContext(r.Context()),
	}
	for _, row := range result.Rows {
		if row.SkipReason == "" {
			view.ImportableRows = append(view.ImportableRows, row)
		} else {
			view.SkippedRows = append(view.SkippedRows, row)
		}
	}
	view.Importable = len(view.ImportableRows)
	view.Skipped = len(view.SkippedRows)

	toastType := "success"
	switch {
	case view.Importable == 0:
		toastType = "error"
	case view.Skipped > 0:
		toastType = "warning"
	}
	toastHeader(w, fmt.Sprintf("%d row(s) importable, %d skipped.", view.Importable, view.Skipped), toastType)

	h.renderCSVPartial(w, r, "csvImportPreviewBody", view, view)
}

// handleCSVImportConfirm creates people, birthday events, group memberships and
// tags from a previewed payload. It is additive only and idempotent: a name
// that already exists (from a previous submit or the wider library) is skipped.
func (h *Handler) handleCSVImportConfirm(w http.ResponseWriter, r *http.Request) {
	payload := r.FormValue("payload")
	if payload == "" {
		h.csvImportMessage(w, r, "No import data provided.")
		return
	}
	var rows []csvimport.Row
	if err := json.Unmarshal([]byte(payload), &rows); err != nil {
		slog.Error("csv import: decode payload", "error", err)
		h.csvImportMessage(w, r, "Invalid import payload.")
		return
	}
	if len(rows) == 0 {
		h.csvImportMessage(w, r, "No import data provided.")
		return
	}

	ctx := r.Context()
	existing := h.existingPersonNames(r)

	var imported, skipped, birthdays, groupLinks, tagLinks int
	for _, row := range rows {
		// Hard skip reasons are never retried; "duplicate of existing person"
		// is re-evaluated below against the current library so a re-submit
		// behaves sensibly.
		if row.SkipReason != "" && row.SkipReason != csvimport.ReasonDuplicateExisting {
			skipped++
			continue
		}
		name := strings.TrimSpace(row.Name)
		if name == "" {
			skipped++
			continue
		}
		key := csvimport.NameKey(name)
		if existing[key] {
			skipped++
			continue
		}
		birthday, ok := vcard.ParseBDAY(row.Date)
		if !ok {
			slog.Error("csv import: parse payload date", "name", name, "date", row.Date)
			skipped++
			continue
		}

		p, err := h.people.CreateStructured(ctx, name, row.First, row.Middle, row.Last, row.Notes, "")
		if err != nil {
			slog.Error("csv import: create person", "name", name, "error", err)
			skipped++
			continue
		}
		existing[key] = true
		imported++

		if _, err := h.events.CreateForPerson(ctx, p.ID, "birthday", birthday, "Birthday of "+name); err != nil {
			slog.Error("csv import: create birthday event", "person_id", p.ID, "error", err)
		} else {
			birthdays++
		}

		for _, groupName := range row.Groups {
			groupName = strings.TrimSpace(groupName)
			if groupName == "" {
				continue
			}
			g, err := h.groups.GetOrCreateByName(ctx, groupName)
			if err != nil {
				slog.Warn("csv import: ensure group", "group", groupName, "error", err)
				continue
			}
			if err := h.groups.AddPerson(ctx, g.ID, p.ID); err != nil {
				slog.Warn("csv import: add person to group", "group", groupName, "error", err)
				continue
			}
			groupLinks++
		}

		for _, tagName := range row.Tags {
			tagName = strings.TrimSpace(tagName)
			if tagName == "" {
				continue
			}
			if err := h.tags.AddToPerson(ctx, p.ID, tagName); err != nil {
				slog.Warn("csv import: add tag", "tag", tagName, "error", err)
				continue
			}
			tagLinks++
		}
	}

	results := csvImportResults{
		Imported:  imported,
		Skipped:   skipped,
		Birthdays: birthdays,
		Groups:    groupLinks,
		Tags:      tagLinks,
	}

	message := fmt.Sprintf("Imported %d person(s). %d skipped. %d birthday event(s) created.",
		imported, skipped, birthdays)
	toastType := "success"
	if imported == 0 {
		toastType = "error"
	}
	toastHeader(w, message, toastType)

	if r.Header.Get("HX-Request") == "true" {
		h.renderCSVPartial(w, r, "csvImportResults", csvImportView{}, results)
		return
	}
	http.Redirect(w, r, "/people?success="+url.QueryEscape(message), http.StatusSeeOther)
}

// existingPersonNames returns the library's person names keyed by NameKey.
func (h *Handler) existingPersonNames(r *http.Request) map[string]bool {
	names := make(map[string]bool)
	people, err := h.people.List(r.Context())
	if err != nil {
		slog.Error("csv import: list people", "error", err)
		return names
	}
	for _, p := range people {
		names[csvimport.NameKey(p.Name)] = true
	}
	return names
}

// existingGroupNames returns the library's group names keyed by NameKey.
func (h *Handler) existingGroupNames(r *http.Request) map[string]bool {
	names := make(map[string]bool)
	groups, err := h.groups.List(r.Context())
	if err != nil {
		slog.Error("csv import: list groups", "error", err)
		return names
	}
	for _, g := range groups {
		names[csvimport.NameKey(g.Name)] = true
	}
	return names
}

// csvFileAllowed accepts .csv/.txt filenames and the text/csv content type.
func csvFileAllowed(name, contentType string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".csv", ".txt":
		return true
	}
	return strings.Contains(strings.ToLower(contentType), "text/csv")
}
