package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// pushVAPIDPublicKey returns the VAPID public key as JSON so the browser can
// subscribe. Returns 400 when Web Push is not configured.
func (h *Handler) pushVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	if !h.notifReg.IsConfigured("webpush") || h.cfg.PushVAPIDPublicKey == "" {
		writeJSONError(w, http.StatusBadRequest, "Web Push is not configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"public_key": h.cfg.PushVAPIDPublicKey,
	})
}

// pushSubscribe stores a browser push subscription for the current user.
// Re-subscribing with the same endpoint replaces the stored keys.
func (h *Handler) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Endpoint string `json:"endpoint"`
		P256dh   string `json:"p256dh"`
		Auth     string `json:"auth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Endpoint == "" || req.P256dh == "" || req.Auth == "" {
		writeJSONError(w, http.StatusBadRequest, "endpoint, p256dh and auth are required")
		return
	}

	if _, err := h.pushSubs.Upsert(r.Context(), userID, req.Endpoint, req.P256dh, req.Auth); err != nil {
		slog.Error("push: store subscription", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to store subscription")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "subscribed"})
}

// pushUnsubscribe removes a browser push subscription by endpoint.
func (h *Handler) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Endpoint == "" {
		writeJSONError(w, http.StatusBadRequest, "endpoint is required")
		return
	}

	if err := h.pushSubs.DeleteByEndpoint(r.Context(), req.Endpoint); err != nil {
		slog.Error("push: remove subscription", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to remove subscription")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}

// pushServiceWorker serves the service worker from the origin root so its
// scope covers the whole app (required for notifications to work on any page).
func (h *Handler) pushServiceWorker(w http.ResponseWriter, r *http.Request) {
	sw, err := staticFS.ReadFile("static/sw.js")
	if err != nil {
		slog.Error("push: read service worker", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "service worker unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(sw)
}

// userID returns the authenticated user's ID.
func (h *Handler) userID(r *http.Request) (int, bool) {
	u := UserFromContext(r.Context())
	if u == nil {
		return 0, false
	}
	return u.ID, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("push: encode response", "error", err)
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
