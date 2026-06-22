package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/datey/datey/ent"
)

var Version = "dev"

// HealthCheck returns basic health status.
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"time":    time.Now().Format(time.RFC3339),
		"version": Version,
	}); err != nil {
		slog.Error("health check: encode response", "error", err)
	}
}

// DBHealthCheck returns health status including database connectivity.
func DBHealthCheck(client *ent.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		status := "ok"
		dbStatus := "ok"

		// Test database connectivity with a simple query
		_, err := client.User.Query().Count(ctx)
		if err != nil {
			dbStatus = "error"
			status = "degraded"
		}

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"status":     status,
			"time":       time.Now().Format(time.RFC3339),
			"version":    Version,
			"database":   dbStatus,
		}

		if dbStatus == "error" {
			slog.Error("db health check: database error", "error", err)
			resp["database_error"] = "database connection failed"
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("db health check: encode response", "error", err)
		}
	}
}
