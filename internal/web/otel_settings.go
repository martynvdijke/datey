package web

import (
	"net/http"
	"strings"
)

// otelSettings is a public JSON endpoint used by the mobile app to discover
// the OTLP collector endpoint at runtime. It exposes no secrets: only the
// endpoint URL and which signal types are enabled.
func (h *Handler) otelSettings(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.TrimSpace(h.cfg.OTLPEndpoint)
	if row, err := h.settingsStore.Current(r.Context()); err == nil && row.OtelEndpoint != nil {
		endpoint = strings.TrimSpace(*row.OtelEndpoint)
	}
	enabled := endpoint != ""
	writeAPIJSON(w, http.StatusOK, map[string]any{
		"endpoint":        endpoint,
		"enabled":         enabled,
		"traces_enabled":  enabled,
		"metrics_enabled": false,
		"logs_enabled":    false,
	})
}
