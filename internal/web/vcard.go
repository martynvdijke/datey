package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/vcard"
	"github.com/go-chi/chi/v5"
)

const maxUploadSize = 10 << 20 // 10 MB

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

	var imported, skipped int
	for _, pc := range parsed {
		// Duplicate check: skip if person with this name already exists
		existing, err := h.people.FindByName(r.Context(), pc.Name)
		if err == nil && existing != nil {
			skipped++
			continue
		}
		if !ent.IsNotFound(err) && err != nil {
			slog.Error("import vcard: check duplicate", "name", pc.Name, "error", err)
			skipped++
			continue
		}

		_, err = h.people.Create(r.Context(), pc.Name, pc.Notes)
		if err != nil {
			slog.Error("import vcard: create person", "name", pc.Name, "error", err)
			skipped++
			continue
		}
		imported++
	}

	msg := fmt.Sprintf("Imported+%d+person(s).+%d+skipped.", imported, skipped)
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
