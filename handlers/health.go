package handlers

import (
	"encoding/json"
	"net/http"
	"time"
)

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"time":    time.Now().Format(time.RFC3339),
		"version": "1.0.1",
	})
}
