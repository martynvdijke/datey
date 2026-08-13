package web

import (
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"

	"github.com/datey/datey/internal/carddav"
	"github.com/datey/datey/internal/settings"
)

// carddavView carries the renderable CardDAV configuration for the
// Settings → Notifications tab. HasPassword is true when a password is already
// stored so the form can show a "leave blank to keep" placeholder instead of
// echoing the secret.
type carddavView struct {
	Enabled      bool
	URL          string
	Username     string
	HasPassword  bool
	DeletePolicy string
	Errors       map[string]string
}

// settingsCarddavSave handles POST /settings/carddav. It validates and
// persists the CardDAV configuration and re-renders the notifications tab with
// inline errors on validation failure.
func (h *Handler) settingsCarddavSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	errs, err := h.settingsStore.ApplyCarddavForm(r.Context(), h.cfg, r.PostForm)
	if err != nil && err != settings.ErrInvalid {
		slog.Error("save carddav settings", "error", err)
		http.Error(w, "failed to save carddav settings", http.StatusInternalServerError)
		return
	}
	if len(errs) > 0 {
		ctx := contextWithFormErrors(r.Context(), errs)
		req := r.WithContext(ctx)
		req.Method = http.MethodPost
		h.settings(w, req)
		return
	}

	toastHeader(w, "CardDAV settings saved", "success")
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// settingsCarddavSync handles POST /settings/carddav/sync — a manual "Sync
// now" that runs a full pull+push regardless of the daily rate limit. The
// result (including per-error details) is rendered into the #carddav-result
// target; the syncer also logs every error and the summary via slog, which the
// logstore captures for the Settings → Logs viewer.
func (h *Handler) settingsCarddavSync(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.CarddavEnabled || h.cfg.CarddavURL == "" {
		if _, err := w.Write([]byte(`<div class="alert alert-warning">CardDAV sync is not enabled. Enable and configure it in the form above first.</div>`)); err != nil {
			slog.Error("write response", "error", err)
		}
		return
	}

	syncer := carddav.NewSyncer(h.cfg, h.client, h.settingsStore)
	res, err := syncer.Sync(r.Context(), carddav.SyncFull, true)
	if err != nil {
		slog.Error("carddav manual sync failed", "error", err)
		if _, err := w.Write([]byte(`<div class="alert alert-danger">CardDAV sync failed: ` + html.EscapeString(err.Error()) + `</div>`)); err != nil {
			slog.Error("write response", "error", err)
		}
		return
	}

	var b strings.Builder
	b.WriteString(`<div class="alert alert-success mb-0">`)
	fmt.Fprintf(&b, "CardDAV sync complete. Pulled: %d created, %d updated, %d deleted. Pushed: %d created, %d updated, %d deleted.",
		res.PulledCreated, res.PulledUpdated, res.PulledDeleted,
		res.PushedCreated, res.PushedUpdated, res.PushedDeleted)
	if len(res.Errors) > 0 {
		b.WriteString(`<ul class="mb-0 mt-1">`)
		for _, e := range res.Errors {
			b.WriteString(`<li>` + html.EscapeString(e) + `</li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</div>`)
	if _, err := w.Write([]byte(b.String())); err != nil {
		slog.Error("write response", "error", err)
	}
}
