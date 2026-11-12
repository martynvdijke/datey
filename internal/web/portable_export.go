package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/datey/datey/internal/i18n"
	"github.com/datey/datey/internal/portable"
)

// portableMaxUploadSize caps a portable JSON upload. A full library export is
// far smaller than this; the cap keeps a bad upload harmless.
const portableMaxUploadSize = 10 << 20

// portableDefaults is a render-time fallback for the template's i18n keys so
// the page is readable before the keys land in the locale catalogs. i18n.T
// still wins as soon as a catalog entry exists.
var portableDefaults = map[string]string{
	"portable.title":           "Portable data export",
	"portable.help":            "Download one human-readable JSON document with people, groups, events, gift ideas, tags, relationships, notes and recurring rules. Credentials and internal identifiers are never included.",
	"portable.export":          "Export JSON",
	"portable.export_hint":     "The download is a dated JSON file you can keep, diff or import into another Datey instance.",
	"portable.import_heading":  "Import JSON",
	"portable.file_label":      "Portable JSON file",
	"portable.file_hint":       "Upload a file previously exported from Datey. Import is additive: existing records are never changed or deleted.",
	"portable.preview":         "Preview import",
	"portable.problems":        "The file could not be used",
	"portable.summary":         "What this import would do",
	"portable.entity":          "Records",
	"portable.new":             "New",
	"portable.skip":            "Already present",
	"portable.conflict":        "Different content, will be kept as-is",
	"portable.people":          "People",
	"portable.groups":          "Groups",
	"portable.tags":            "Tags",
	"portable.events":          "Events",
	"portable.gift_ideas":      "Gift ideas",
	"portable.relationships":   "Relationships",
	"portable.person_notes":    "Person notes",
	"portable.group_notes":     "Group notes",
	"portable.recurring_rules": "Recurring rules",
	"portable.confirm":         "Import now",
	"portable.confirm_hint":    "Nothing has been saved yet. Only missing records are created.",
	"portable.results":         "Import result",
	"portable.created":         "Created",
	"portable.skipped":         "Skipped",
	"portable.skipped_reasons": "Skipped records",
	"portable.reason":          "Reason",
	"portable.back":            "Back",
	"portable.no_preview":      "Choose a portable JSON file to see what would be imported.",
	"portable.import_again":    "Import another file",
}

// portableEntityRow is one line in the preview's counts table.
type portableEntityRow struct {
	Key    string
	Counts portable.Counts
}

// portableResultRow is one line in the result summary.
type portableResultRow struct {
	Key    string
	Result portable.EntityResult
}

// portableView is the data for the upload, preview and result pages.
type portableView struct {
	HasPreview   bool
	Rows         []portableEntityRow
	ResultRows   []portableResultRow
	Problems     []string
	EnvelopeJSON string
	Result       *portable.Result
	CSRFToken    string
}

var (
	portableTemplateOnce sync.Once
	portableTemplate     *template.Template
	portableTemplateErr  error
)

// portableTemplateFile loads the portable import page with the shared base
// layout, mirroring the CSV import page's standalone loader.
func portableTemplateFile() (*template.Template, error) {
	portableTemplateOnce.Do(func() {
		portableTemplate, portableTemplateErr = template.New("base.html").
			Funcs(funcMap).
			ParseFS(templateFS, "templates/base.html", "templates/portable_import.html")
	})
	return portableTemplate, portableTemplateErr
}

func portableTranslator(locale string) func(string) string {
	return func(key string) string {
		if v := i18n.T(locale, key); v != key {
			return v
		}
		if v, ok := portableDefaults[key]; ok {
			return v
		}
		return key
	}
}

// renderPortablePage renders the full page with an explicit status code so a
// rejected upload can still show a friendly explanation.
func (h *Handler) renderPortablePage(w http.ResponseWriter, r *http.Request, status int, view portableView) {
	tmpl, err := portableTemplateFile()
	if err != nil {
		slog.Error("portable import: load template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned, err := tmpl.Clone()
	if err != nil {
		slog.Error("portable import: clone template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned.Funcs(template.FuncMap{"T": portableTranslator(i18n.LocaleFromContext(r.Context()))})

	title := "Datey - Portable data"
	data := h.baseData(r, title)
	view.CSRFToken = csrfTokenFromContext(r.Context())
	data["Title"] = title
	data["View"] = view

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := cloned.ExecuteTemplate(w, "base.html", data); err != nil {
		slog.Error("portable import: render page", "error", err)
	}
}

// portableDeps wires the portable package to the handler's repositories.
func (h *Handler) portableDeps() portable.Deps {
	return portable.Deps{
		People:        h.people,
		Groups:        h.groups,
		Events:        h.events,
		GiftIdeas:     h.giftIdeas,
		Tags:          h.tags,
		Relationships: h.relationships,
		PersonNotes:   h.personNotes,
		GroupNotes:    h.groupNotes,
		Rules:         h.recurringRules,
		Source:        portable.Source{AppURL: h.cfg.AppURL},
	}
}

// handlePortableExport streams the whole portable document as a JSON
// attachment. It is available to any authenticated user.
func (h *Handler) handlePortableExport(w http.ResponseWriter, r *http.Request) {
	env, err := portable.Export(r.Context(), h.portableDeps())
	if err != nil {
		slog.Error("portable export: build envelope", "error", err)
		http.Error(w, "could not build export", http.StatusInternalServerError)
		return
	}
	body, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		slog.Error("portable export: marshal envelope", "error", err)
		http.Error(w, "could not build export", http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("datey-portable-%s.json", time.Now().UTC().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// handlePortableImportForm renders the empty upload page.
func (h *Handler) handlePortableImportForm(w http.ResponseWriter, r *http.Request) {
	h.renderPortablePage(w, r, http.StatusOK, portableView{})
}

// handlePortableImportPreview parses an uploaded document, validates it and
// shows what an import would create. Nothing is written here.
func (h *Handler) handlePortableImportPreview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, portableMaxUploadSize)
	if err := r.ParseMultipartForm(portableMaxUploadSize); err != nil {
		slog.Error("portable import: parse multipart form", "error", err)
		h.renderPortablePage(w, r, http.StatusBadRequest, portableView{
			Problems: []string{"The upload is not a usable multipart form. The limit is 10 MB."},
		})
		return
	}

	file, header, err := r.FormFile("portable_file")
	if err != nil {
		h.renderPortablePage(w, r, http.StatusBadRequest, portableView{
			Problems: []string{"No file was uploaded."},
		})
		return
	}
	defer func() { _ = file.Close() }()

	if header.Size > portableMaxUploadSize {
		h.renderPortablePage(w, r, http.StatusBadRequest, portableView{
			Problems: []string{"File too large. The limit is 10 MB."},
		})
		return
	}
	if !strings.EqualFold(filepath.Ext(header.Filename), ".json") {
		h.renderPortablePage(w, r, http.StatusBadRequest, portableView{
			Problems: []string{"Only .json files are supported."},
		})
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, portableMaxUploadSize+1))
	if err != nil {
		slog.Error("portable import: read upload", "error", err)
		h.renderPortablePage(w, r, http.StatusBadRequest, portableView{
			Problems: []string{"Could not read the uploaded file."},
		})
		return
	}
	if len(data) > portableMaxUploadSize {
		h.renderPortablePage(w, r, http.StatusBadRequest, portableView{
			Problems: []string{"File too large. The limit is 10 MB."},
		})
		return
	}

	env, err := portable.Parse(data)
	if err != nil {
		h.renderPortablePage(w, r, http.StatusBadRequest, portableView{
			Problems: validationProblems(err),
		})
		return
	}
	existing, err := portable.LoadExisting(r.Context(), h.portableDeps())
	if err != nil {
		slog.Error("portable import: load existing", "error", err)
		h.renderPortablePage(w, r, http.StatusInternalServerError, portableView{
			Problems: []string{"Could not read the current library."},
		})
		return
	}
	preview := portable.Preview(existing, env)
	encoded, err := json.Marshal(env)
	if err != nil {
		slog.Error("portable import: marshal envelope", "error", err)
		h.renderPortablePage(w, r, http.StatusInternalServerError, portableView{
			Problems: []string{"Could not prepare the import."},
		})
		return
	}

	h.renderPortablePage(w, r, http.StatusOK, portableView{
		HasPreview:   true,
		Rows:         previewRows(preview),
		EnvelopeJSON: string(encoded),
	})
}

// handlePortableImportConfirm imports the submitted envelope. The document is
// re-parsed from the form field (never from the filesystem) and validated
// again before anything is written. A repeat confirm creates nothing.
func (h *Handler) handlePortableImportConfirm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPortablePage(w, r, http.StatusBadRequest, portableView{
			Problems: []string{"Could not read the submitted form."},
		})
		return
	}
	raw := r.FormValue("envelope")
	if strings.TrimSpace(raw) == "" {
		h.renderPortablePage(w, r, http.StatusBadRequest, portableView{
			Problems: []string{"No import data was provided."},
		})
		return
	}
	env, err := portable.Parse([]byte(raw))
	if err != nil {
		h.renderPortablePage(w, r, http.StatusBadRequest, portableView{
			Problems: validationProblems(err),
		})
		return
	}
	result, err := portable.Import(r.Context(), h.portableDeps(), env, portable.Options{})
	if err != nil {
		slog.Error("portable import: import", "error", err)
		h.renderPortablePage(w, r, http.StatusInternalServerError, portableView{
			Problems: validationProblems(err),
		})
		return
	}
	h.renderPortablePage(w, r, http.StatusOK, portableView{
		Result:     &result,
		ResultRows: resultRows(result),
	})
}

// validationProblems unwraps a ValidationError into its individual messages so
// the page can list every problem at once.
func validationProblems(err error) []string {
	var ve *portable.ValidationError
	if errors.As(err, &ve) && len(ve.Problems) > 0 {
		return ve.Problems
	}
	return []string{err.Error()}
}

func previewRows(p portable.PreviewResult) []portableEntityRow {
	return []portableEntityRow{
		{Key: "portable.people", Counts: p.People},
		{Key: "portable.groups", Counts: p.Groups},
		{Key: "portable.tags", Counts: p.Tags},
		{Key: "portable.events", Counts: p.Events},
		{Key: "portable.gift_ideas", Counts: p.GiftIdeas},
		{Key: "portable.relationships", Counts: p.Relationships},
		{Key: "portable.person_notes", Counts: p.PersonNotes},
		{Key: "portable.group_notes", Counts: p.GroupNotes},
		{Key: "portable.recurring_rules", Counts: p.RecurringRules},
	}
}

func resultRows(r portable.Result) []portableResultRow {
	return []portableResultRow{
		{Key: "portable.people", Result: r.People},
		{Key: "portable.groups", Result: r.Groups},
		{Key: "portable.tags", Result: r.Tags},
		{Key: "portable.events", Result: r.Events},
		{Key: "portable.gift_ideas", Result: r.GiftIdeas},
		{Key: "portable.relationships", Result: r.Relationships},
		{Key: "portable.person_notes", Result: r.PersonNotes},
		{Key: "portable.group_notes", Result: r.GroupNotes},
		{Key: "portable.recurring_rules", Result: r.RecurringRules},
	}
}
