package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/datey/datey/internal/config"
)

func TestMatrixNotifier_Name(t *testing.T) {
	n := NewMatrixNotifier(&config.Config{})
	if got := n.Name(); got != "matrix" {
		t.Errorf("Name() = %q, want matrix", got)
	}
}

func TestMatrixNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"all set", &config.Config{MatrixHomeserverURL: "https://matrix.example.com", MatrixAccessToken: "tok", MatrixRoomID: "!abc:example.com"}, true},
		{"missing homeserver", &config.Config{MatrixHomeserverURL: "", MatrixAccessToken: "tok", MatrixRoomID: "!abc:example.com"}, false},
		{"missing token", &config.Config{MatrixHomeserverURL: "https://matrix.example.com", MatrixAccessToken: "", MatrixRoomID: "!abc:example.com"}, false},
		{"missing room", &config.Config{MatrixHomeserverURL: "https://matrix.example.com", MatrixAccessToken: "tok", MatrixRoomID: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewMatrixNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatrixNotifier_Send_PUTAndAuth(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"event_id":"$abc"}`))
	}))
	defer srv.Close()

	n := NewMatrixNotifier(&config.Config{
		MatrixHomeserverURL: srv.URL,
		MatrixAccessToken:   "secret-token",
		MatrixRoomID:        "!room123:example.com",
	})
	if err := n.Send(context.Background(), "Reminder: Dana - birthday", "Dana's birthday is in 3 days"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("auth = %q", gotAuth)
	}
	if !strings.Contains(gotPath, "/_matrix/client/v3/rooms/") || !strings.Contains(gotPath, "/send/m.room.message/") {
		t.Errorf("path = %q, want matrix send path", gotPath)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if payload["msgtype"] != "m.text" {
		t.Errorf("msgtype = %v", payload["msgtype"])
	}
	if payload["body"] != "Reminder: Dana - birthday\nDana's birthday is in 3 days" {
		t.Errorf("body = %v", payload["body"])
	}
	if payload["format"] != "org.matrix.custom.html" {
		t.Errorf("format = %v", payload["format"])
	}
	if payload["formatted_body"] == nil {
		t.Errorf("formatted_body missing")
	}
}

func TestMatrixNotifier_Send_MForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errcode":"M_FORBIDDEN","error":"You are not in this room"}`))
	}))
	defer srv.Close()
	n := NewMatrixNotifier(&config.Config{
		MatrixHomeserverURL: srv.URL,
		MatrixAccessToken:   "tok",
		MatrixRoomID:        "!abc:example.com",
	})
	err := n.Send(context.Background(), "t", "m")
	if err == nil {
		t.Fatal("expected M_FORBIDDEN error")
	}
	if !strings.Contains(err.Error(), "M_FORBIDDEN") {
		t.Errorf("error = %q, want M_FORBIDDEN", err)
	}
}

func TestMatrixNotifier_Send_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	n := NewMatrixNotifier(&config.Config{
		MatrixHomeserverURL: srv.URL,
		MatrixAccessToken:   "tok",
		MatrixRoomID:        "!abc:example.com",
	})
	err := n.Send(context.Background(), "t", "m")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}
