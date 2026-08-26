package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/vcard"
	"github.com/go-chi/chi/v5"
)

const maxUploadSize = 10 << 20 // 10 MB

type importResult struct {
	Name            string
	BirthdayCreated bool
	HasBirthdayBday string // formatted birthday date, empty if none
	Created         bool
	Updated         bool   // true when an existing person was overwritten
	SkipReason      string // empty if created
	BirthdayWarning string // non-empty when a BDAY value could not be parsed
}

func (h *Handler) handleImportVCard(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		slog.Error("import vcard: parse multipart form", "error", err)
		http.Redirect(w, r, "/people?error=File+too+large+or+invalid+form", http.StatusSeeOther)
		return
	}

	files := r.MultipartForm.File["vcf_file"]
	if len(files) == 0 {
		slog.Error("import vcard: no uploaded files")
		http.Redirect(w, r, "/people?error=No+file+uploaded", http.StatusSeeOther)
		return
	}

	overwrite := r.FormValue("overwrite") == "true"

	var imported, skipped, updated, birthdays int
	results := make([]importResult, 0)

	for _, fh := range files {
		file, err := fh.Open()
		if err != nil {
			slog.Error("import vcard: open uploaded file", "file", fh.Filename, "error", err)
			skipped++
			results = append(results, importResult{
				Name:       fh.Filename,
				Created:    false,
				SkipReason: "Invalid file",
			})
			continue
		}

		parsed, err := vcard.Parse(file)
		_ = file.Close()
		if err != nil {
			slog.Error("import vcard: parse file", "file", fh.Filename, "error", err)
			skipped++
			results = append(results, importResult{
				Name:       fh.Filename,
				Created:    false,
				SkipReason: "Invalid file",
			})
			continue
		}

		if len(parsed) == 0 {
			skipped++
			results = append(results, importResult{
				Name:       fh.Filename,
				Created:    false,
				SkipReason: "No people found",
			})
			continue
		}

		for _, pc := range parsed {
			existing, err := h.people.FindByName(r.Context(), pc.Name)
			if err == nil && existing != nil {
				if overwrite {
					person, err := h.people.UpdateStructured(r.Context(), existing.ID, pc.Name, pc.GivenName, pc.MiddleName, pc.FamilyName, pc.Notes, pc.RawData)
					if err != nil {
						slog.Error("import vcard: update person", "name", pc.Name, "error", err)
						skipped++
						results = append(results, importResult{
							Name:       pc.Name,
							Created:    false,
							SkipReason: "Update error",
						})
						continue
					}
					updated++
					ir := importResult{
						Name:    pc.Name,
						Created: false,
						Updated: true,
					}
					if h.maybeCreateBirthdayEvent(r.Context(), person.ID, pc, &ir) {
						birthdays++
					}
					// Sync group membership from CATEGORIES (create missing groups, degrade gracefully on error).
					h.syncCategoriesToGroups(r.Context(), person.ID, pc.Categories)
					results = append(results, ir)
				} else {
					skipped++
					results = append(results, importResult{
						Name:       pc.Name,
						Created:    false,
						SkipReason: "Duplicate name",
					})
				}
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

			person, err := h.people.CreateStructured(r.Context(), pc.Name, pc.GivenName, pc.MiddleName, pc.FamilyName, pc.Notes, pc.RawData)
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
			if h.maybeCreateBirthdayEvent(r.Context(), person.ID, pc, &ir) {
				birthdays++
			}
			h.syncCategoriesToGroups(r.Context(), person.ID, pc.Categories)
			results = append(results, ir)
		}
	}

	// HTMX request — return inline results partial + toast.
	if r.Header.Get("HX-Request") == "true" {
		isHX := true
		toastMsg := fmt.Sprintf("Imported %d person(s). %d skipped.", imported, skipped)
		if updated > 0 {
			toastMsg = fmt.Sprintf("Imported %d person(s). %d updated. %d skipped.", imported, updated, skipped)
		}
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
			"Updated":       updated,
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
	if updated > 0 {
		msg = fmt.Sprintf("Imported+%d+person(s).+%d+updated.+%d+skipped.", imported, updated, skipped)
	}
	if birthdays > 0 {
		msg += fmt.Sprintf("+%d+birthday+event(s)+created.", birthdays)
	}
	http.Redirect(w, r, "/people?success="+msg, http.StatusSeeOther)
}

// syncCategoriesToGroups maps CATEGORIES to group membership, creating missing
// groups by name. Where full fidelity isn't possible (e.g. empty name, DB error),
// it degrades gracefully: errors are logged and the import continues without
// failing the person creation.
func (h *Handler) syncCategoriesToGroups(ctx context.Context, personID int, categories []string) {
	for _, name := range categories {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		g, err := h.groups.GetOrCreateByName(ctx, name)
		if err != nil {
			slog.Warn("import vcard: ensure group for category", "category", name, "error", err)
			continue
		}
		if err := h.groups.AddPerson(ctx, g.ID, personID); err != nil {
			// AddPerson is idempotent via ent edge; log but don't fail.
			slog.Warn("import vcard: add person to group", "group", name, "error", err)
		}
	}
}

// maybeCreateBirthdayEvent creates a birthday event from the parsed contact when
// it has a BDAY and the person does not already have a birthday event. It fills
// ir.HasBirthdayBday and reports whether a new event was created.
func (h *Handler) maybeCreateBirthdayEvent(ctx context.Context, personID int, pc vcard.ParsedContact, ir *importResult) bool {
	if pc.BirthdayParseFailed {
		ir.BirthdayWarning = "Could not parse birthday"
	}
	if pc.Birthday == nil {
		return false
	}
	ir.HasBirthdayBday = formatEventDate(h.cfg.DateVariant, *pc.Birthday)

	// Dedup: never create a second birthday event for the same person.
	existingEvents, err := h.events.ListByPerson(ctx, personID)
	if err == nil {
		for _, ev := range existingEvents {
			if ev.Type == "birthday" {
				return false
			}
		}
	}

	desc := fmt.Sprintf("Birthday of %s", pc.Name)
	if _, err := h.events.CreateForPerson(ctx, personID, "birthday", *pc.Birthday, desc); err != nil {
		slog.Error("import vcard: create birthday event", "name", pc.Name, "error", err)
		return false
	}
	ir.BirthdayCreated = true
	return true
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

	// Include group membership as CATEGORIES.
	var cats []string
	if groups, err2 := h.groups.ListByPerson(r.Context(), id); err2 == nil {
		for _, g := range groups {
			cats = append(cats, g.Name)
		}
	}
	data, err := vcard.EncodeSingleWithCategories(person.Name, person.Notes, cats)
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
		var cats []string
		if groups, err2 := h.groups.ListByPerson(r.Context(), p.ID); err2 == nil {
			for _, g := range groups {
				cats = append(cats, g.Name)
			}
		}
		items[i] = vcard.NameNotes{Name: p.Name, Notes: p.Notes, Categories: cats}
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
