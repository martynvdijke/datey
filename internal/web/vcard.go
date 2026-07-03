package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/vcard"
	"github.com/go-chi/chi/v5"
)

const maxUploadSize = 10 << 20 // 10 MB

type importResult struct {
	Name             string
	BirthdayCreated  bool
	HasBirthdayBday  string // formatted birthday date, empty if none
	Created          bool
	SkipReason       string // empty if created
}

func (h *Handler) handleImportVCard(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		slog.Error("import vcard: parse multipart form", "error", err)
		http.Redirect(w, r, "/people?error=File+too+large+or+invalid+form", http.StatusSeeOther)
		return
	}

	file, _, err := r.FormFile("vcf_file")
	if err != nil {
		slog.Error("import vcard: get uploaded file", "error", err)
		http.Redirect(w, r, "/people?error=No+file+uploaded", http.StatusSeeOther)
		return
	}
	defer func() { _ = file.Close() }()

	parsed, err := vcard.Parse(file)
	if err != nil {
		slog.Error("import vcard: parse", "error", err)
		http.Redirect(w, r, "/people?error=Invalid+vCard+file", http.StatusSeeOther)
		return
	}

	if len(parsed) == 0 {
		http.Redirect(w, r, "/people?error=No+people+found+in+the+uploaded+file", http.StatusSeeOther)
		return
	}

	var imported, skipped, birthdays int
	results := make([]importResult, 0, len(parsed))

	for _, pc := range parsed {
		existing, err := h.people.FindByName(r.Context(), pc.Name)
		if err == nil && existing != nil {
			skipped++
			results = append(results, importResult{
				Name:       pc.Name,
				Created:    false,
				SkipReason: "Duplicate name",
			})
			continue
		}
		if !ent.IsNotFound(err) && err != nil {
			slog.Error("import vcard: check duplicate", "name", pc.Name, "error", err)
			skipped++
			results = append(results, importResult{
				Name:       pc.Name,
				Created:    false,
				SkipReason: "Lookup error",
			})
			continue
		}

		person, err := h.people.Create(r.Context(), pc.Name, pc.Notes)
		if err != nil {
			slog.Error("import vcard: create person", "name", pc.Name, "error", err)
			skipped++
			results = append(results, importResult{
				Name:       pc.Name,
				Created:    false,
				SkipReason: "Creation error",
			})
			continue
		}

		imported++

		ir := importResult{
			Name:    pc.Name,
			Created: true,
		}

		if pc.Birthday != nil {
			ir.HasBirthdayBday = pc.Birthday.Format("Jan 2, 2006")
			desc := fmt.Sprintf("Birthday of %s", pc.Name)
			if _, err := h.events.CreateForPerson(r.Context(), person.ID, "birthday", *pc.Birthday, desc); err != nil {
				slog.Error("import vcard: create birthday event", "name", pc.Name, "error", err)
			} else {
				birthdays++
				ir.BirthdayCreated = true
			}
		}

		results = append(results, ir)
	}

	// HTMX request — return inline results partial + toast.
	if r.Header.Get("HX-Request") == "true" {
		isHX := true
		toastMsg := fmt.Sprintf("Imported %d person(s). %d skipped.", imported, skipped)
		if birthdays > 0 {
			toastMsg += fmt.Sprintf(" %d birthday event(s) created.", birthdays)
		}
		toastType := "success"
		if imported == 0 {
			toastType = "error"
		}

		data := map[string]any{
			"ImportResults": results,
			"Imported":      imported,
			"Skipped":       skipped,
			"Birthdays":     birthdays,
			"IsHTMX":        isHX,
		}

		// Render just the importResults partial template.
		tmpl := h.templates["people.html"]
		if partial := tmpl.Lookup("importResults"); partial != nil {
			payload := map[string]any{
				"show-toast": map[string]string{
					"message": toastMsg,
					"type":    toastType,
				},
			}
			b, _ := json.Marshal(payload)
			w.Header().Set("HX-Trigger", string(b))
			if err := partial.Execute(w, data); err != nil {
				slog.Error("import vcard: render results", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		} else {
			slog.Error("import vcard: importResults template not found")
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	// Non-HTMX fallback: redirect with query-param message.
	msg := fmt.Sprintf("Imported+%d+person(s).+%d+skipped.", imported, skipped)
	if birthdays > 0 {
		msg += fmt.Sprintf("+%d+birthday+event(s)+created.", birthdays)
	}
	http.Redirect(w, r, "/people?success="+msg, http.StatusSeeOther)
}

func (h *Handler) handleExportSingleVCard(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	person, err := h.people.Get(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("export vcard: get person", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data, err := vcard.EncodeSingle(person.Name, person.Notes)
	if err != nil {
		slog.Error("export vcard: encode single", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	filename := vcard.SanitizeFilename(person.Name) + ".vcf"
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if _, err := w.Write(data); err != nil {
		slog.Error("write vcard response", "error", err)
	}
}

func (h *Handler) handleExportAllVCard(w http.ResponseWriter, r *http.Request) {
	people, err := h.people.List(r.Context())
	if err != nil {
		slog.Error("export all vcard: list people", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	items := make([]vcard.NameNotes, len(people))
	for i, p := range people {
		items[i] = vcard.NameNotes{Name: p.Name, Notes: p.Notes}
	}
	data, err := vcard.Encode(items)
	if err != nil {
		slog.Error("export all vcard: encode", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="datey-contacts.vcf"`)
	if _, err := w.Write(data); err != nil {
		slog.Error("write vcard response", "error", err)
	}
}


