package web

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/immich"
	"github.com/datey/datey/internal/photos"
)

// syncMatchedRow is a person that resolved to an Immich person.
type syncMatchedRow struct {
	PersonID   int
	Name       string
	ImmichID   string
	ImmichName string
	Imported   bool
	Skipped    string // non-empty when the import was intentionally skipped
	Error      string // non-empty when the import failed
}

// syncUnmatchedRow is a person with no usable Immich counterpart.
type syncUnmatchedRow struct {
	PersonID int
	Name     string
	Reason   string // "no match", "ambiguous", "disabled", "override not found"
}

type syncResult struct {
	Total      int
	Matched    []syncMatchedRow
	Unmatched  []syncUnmatchedRow
	ImportedN  int
	SkippedN   int
	FailedN    int
	UnmatchedN int
}

// resolveImmichPerson applies the override-first precedence: an explicit
// immich_person_id wins; otherwise the normalized exact-name match is used,
// with duplicates reported as ambiguous rather than silently dropped.
func resolveImmichPerson(p *ent.Person, people []immich.Person) (*immich.Person, string) {
	if p.ImmichPersonID != nil && *p.ImmichPersonID != "" {
		for i := range people {
			if people[i].ID == *p.ImmichPersonID {
				return &people[i], ""
			}
		}
		return nil, "override not found"
	}
	want := immich.NormalizeName(p.Name)
	var match *immich.Person
	count := 0
	for i := range people {
		if immich.NormalizeName(people[i].Name) == want {
			count++
			match = &people[i]
		}
	}
	switch {
	case count == 0:
		return nil, "no match"
	case count > 1:
		return nil, "ambiguous"
	}
	return match, ""
}

// importImmichPhoto is the shared import routine: download the Immich
// thumbnail, validate it, store it atomically, and record photo state with
// source "immich". On failure any written file is removed and existing photo
// state is untouched.
func (h *Handler) importImmichPhoto(ctx context.Context, p *ent.Person, ip *immich.Person) error {
	body, contentType, err := h.immich.Thumbnail(ctx, ip.ID)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	data, err := photos.Validate(contentType, body)
	if err != nil {
		return err
	}
	relPath, err := h.photoStore.Save(p.ID, contentType, data)
	if err != nil {
		return err
	}
	if _, err := h.people.SetPhotoState(ctx, p.ID, relPath, contentType, "immich"); err != nil {
		_ = h.photoStore.Delete(relPath)
		return err
	}
	return nil
}

// immichBulkSync matches every person against the Immich people list in one
// run and imports thumbnails for matched, non-disabled people. The result is
// read-only: overrides happen on each person's settings page.
func (h *Handler) immichBulkSync(w http.ResponseWriter, r *http.Request) {
	if !h.immich.Enabled() {
		toastHeader(w, "Immich is not configured", "error")
		w.WriteHeader(http.StatusOK)
		return
	}
	allPeople, err := h.people.List(r.Context())
	if err != nil {
		slog.Error("immich sync: list people", "error", err)
		toastHeader(w, "Failed to list people", "error")
		w.WriteHeader(http.StatusOK)
		return
	}
	immichPeople, err := h.immich.People(r.Context())
	if err != nil {
		slog.Error("immich sync: list immich people", "error", err)
		toastHeader(w, "Could not reach Immich: "+err.Error(), "error")
		w.WriteHeader(http.StatusOK)
		return
	}

	result := syncResult{Total: len(allPeople)}
	for _, p := range allPeople {
		row := syncMatchedRow{PersonID: p.ID, Name: p.Name}
		if p.ImmichPhotoDisabled {
			result.Unmatched = append(result.Unmatched, syncUnmatchedRow{PersonID: p.ID, Name: p.Name, Reason: "disabled"})
			continue
		}
		ip, reason := resolveImmichPerson(p, immichPeople)
		if ip == nil {
			result.Unmatched = append(result.Unmatched, syncUnmatchedRow{PersonID: p.ID, Name: p.Name, Reason: reason})
			continue
		}
		row.ImmichID = ip.ID
		row.ImmichName = ip.Name
		if p.PhotoSource != nil && *p.PhotoSource == "upload" {
			row.Skipped = "uploaded photo kept"
			result.SkippedN++
			result.Matched = append(result.Matched, row)
			continue
		}
		if err := h.importImmichPhoto(r.Context(), p, ip); err != nil {
			slog.Warn("immich sync: import failed", "person_id", p.ID, "name", p.Name, "error", err)
			row.Error = err.Error()
			result.FailedN++
			result.Matched = append(result.Matched, row)
			continue
		}
		row.Imported = true
		result.ImportedN++
		result.Matched = append(result.Matched, row)
	}
	result.UnmatchedN = len(result.Unmatched)

	slog.Info("immich sync complete",
		"total", result.Total,
		"imported", result.ImportedN,
		"skipped", result.SkippedN,
		"failed", result.FailedN,
		"unmatched", result.UnmatchedN,
	)

	tmpl, ok := h.templates["immich_sync_result.html"]
	if !ok {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "immich_sync_result.html", map[string]any{
		"Result": result,
	}); err != nil {
		slog.Error("render immich sync result", "error", err)
	}
}
