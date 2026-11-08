package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func otelSettingsRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/otel-settings", h.otelSettings)
	return r
}

type otelSettingsBody struct {
	Endpoint       string `json:"endpoint"`
	Enabled        bool   `json:"enabled"`
	TracesEnabled  bool   `json:"traces_enabled"`
	MetricsEnabled bool   `json:"metrics_enabled"`
	LogsEnabled    bool   `json:"logs_enabled"`
}

func decodeOtelSettings(t *testing.T, body []byte) otelSettingsBody {
	t.Helper()
	var parsed otelSettingsBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, body)
	}
	return parsed
}

// TestOtelSettings_PublicNoAuth verifies the route is reachable without a
// session and returns the documented keys with disabled defaults.
func TestOtelSettings_PublicNoAuth(t *testing.T) {
	h := newTestWebHandler(t)

	req := httptest.NewRequest("GET", "/api/otel-settings", nil)
	w := httptest.NewRecorder()
	otelSettingsRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	body := decodeOtelSettings(t, w.Body.Bytes())
	if body.Endpoint != "" {
		t.Errorf("endpoint = %q, want empty", body.Endpoint)
	}
	if body.Enabled || body.TracesEnabled || body.MetricsEnabled || body.LogsEnabled {
		t.Errorf("expected all signals disabled, got %+v", body)
	}
}

// TestOtelSettings_FromAppConfig verifies the endpoint comes from the ent
// AppConfig row and enables traces.
func TestOtelSettings_FromAppConfig(t *testing.T) {
	h := newTestWebHandler(t)
	row, err := h.settingsStore.Current(context.Background())
	if err != nil {
		t.Fatalf("load app_config: %v", err)
	}
	if _, err := h.client.AppConfig.UpdateOneID(row.ID).
		SetOtelEndpoint("https://collector.example.com:4318").
		Save(context.Background()); err != nil {
		t.Fatalf("seed otel_endpoint: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/otel-settings", nil)
	w := httptest.NewRecorder()
	otelSettingsRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeOtelSettings(t, w.Body.Bytes())
	if body.Endpoint != "https://collector.example.com:4318" {
		t.Errorf("endpoint = %q", body.Endpoint)
	}
	if !body.Enabled {
		t.Error("expected enabled = true")
	}
	if !body.TracesEnabled {
		t.Error("expected traces_enabled = true")
	}
	if body.MetricsEnabled || body.LogsEnabled {
		t.Errorf("metrics/logs should stay disabled, got %+v", body)
	}
}
