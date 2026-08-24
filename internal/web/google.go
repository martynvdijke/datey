package web

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"

	"github.com/datey/datey/internal/googlecontacts"
)

type googleView struct {
	Enabled      bool
	ClientID     string
	HasSecret    bool
	HasRefresh   bool
	DeletePolicy string
	LastSync     string
	Errors       map[string]string
}

func (h *Handler) settingsGoogleSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	errs, err := h.settingsStore.ApplyGoogleForm(r.Context(), h.cfg, r.PostForm)
	if err != nil && err.Error() == "invalid settings form" {
		// re-render via settings page
		http.Redirect(w, r, "/settings?tab=notifications&error=Invalid+google+settings", http.StatusSeeOther)
		return
	}
	if len(errs) > 0 {
		http.Redirect(w, r, "/settings?error=Invalid+google+settings", http.StatusSeeOther)
		return
	}
	if err != nil {
		slog.Error("save google settings", "error", err)
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	h.auditRecord(r, "config.save", "google")
	http.Redirect(w, r, "/settings?success=Google+settings+saved", http.StatusSeeOther)
}

func (h *Handler) settingsGoogleAuth(w http.ResponseWriter, r *http.Request) {
	if h.cfg.GoogleClientID == "" || h.cfg.GoogleClientSecret == "" {
		http.Redirect(w, r, "/settings?error=Configure+client+ID+and+secret+first", http.StatusSeeOther)
		return
	}
	// Generate state nonce
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(b)
	if err := h.settingsStore.SetGoogleOAuthState(r.Context(), state); err != nil {
		slog.Error("store oauth state", "error", err)
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	redirectURL := scheme + "://" + r.Host + "/settings/google/callback"
	cfg := googlecontacts.OAuthConfig(h.cfg.GoogleClientID, h.cfg.GoogleClientSecret, redirectURL)
	url := googlecontacts.AuthURL(cfg, state)
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handler) settingsGoogleCallback(w http.ResponseWriter, r *http.Request) {
	// Require admin session (middleware ensures but double-check)
	if !IsAdmin(r.Context()) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		http.Error(w, "missing state or code", http.StatusBadRequest)
		return
	}
	stored, _ := h.settingsStore.GoogleOAuthState(r.Context())
	if stored != "" && stored != state {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	redirectURL := scheme + "://" + r.Host + "/settings/google/callback"
	cfg := googlecontacts.OAuthConfig(h.cfg.GoogleClientID, h.cfg.GoogleClientSecret, redirectURL)
	tok, err := googlecontacts.ExchangeCode(r.Context(), cfg, code)
	if err != nil {
		slog.Error("google oauth exchange", "error", err)
		http.Redirect(w, r, "/settings?error=Google+authorization+failed", http.StatusSeeOther)
		return
	}
	if tok.RefreshToken != "" {
		if err := h.settingsStore.SetGoogleRefreshToken(r.Context(), tok.RefreshToken); err != nil {
			slog.Error("persist refresh token", "error", err)
		}
		h.cfg.GoogleRefreshToken = tok.RefreshToken
	}
	_ = h.settingsStore.SetGoogleOAuthState(r.Context(), "")
	http.Redirect(w, r, "/settings?success=Google+account+connected", http.StatusSeeOther)
}

func (h *Handler) settingsGoogleSync(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.GoogleContactsEnabled {
		http.Error(w, "google sync not enabled", http.StatusBadRequest)
		return
	}
	if h.cfg.GoogleRefreshToken == "" {
		http.Error(w, "google not connected", http.StatusBadRequest)
		return
	}
	// Build client
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	redirectURL := scheme + "://" + r.Host + "/settings/google/callback"
	oauthCfg := googlecontacts.OAuthConfig(h.cfg.GoogleClientID, h.cfg.GoogleClientSecret, redirectURL)
	ts := googlecontacts.TokenSource(r.Context(), oauthCfg, h.cfg.GoogleRefreshToken)
	transport := &googlecontacts.OAuth2Transport{TokenSource: ts}
	client := googlecontacts.New(transport)
	syncer := googlecontacts.NewSyncer(h.cfg, h.client, h.settingsStore, client)
	res, err := syncer.Sync(r.Context())
	if err != nil {
		slog.Error("google sync failed", "error", err)
		http.Redirect(w, r, "/settings?error=Google+sync+failed", http.StatusSeeOther)
		return
	}
	slog.Info("google manual sync", "created", res.Created, "updated", res.Updated)
	h.auditRecord(r, "google.sync", "")
	http.Redirect(w, r, "/settings?success=Google+sync+complete", http.StatusSeeOther)
}
