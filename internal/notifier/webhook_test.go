package notifier

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/datey/datey/internal/config"
)

func TestWebhookNotifier_Name(t *testing.T) {
	n := NewWebhookNotifier(&config.Config{})
	if got := n.Name(); got != "webhook" {
		t.Errorf("Name() = %q, want %q", got, "webhook")
	}
}

func TestWebhookNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"url set", &config.Config{WebhookURL: "https://example.com/hook"}, true},
		{"url empty", &config.Config{WebhookURL: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewWebhookNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebhookNotifier_Send_EnvelopeShape(t *testing.T) {
	var mu sync.Mutex
	var gotPath, gotMethod, gotCT, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(&config.Config{WebhookURL: srv.URL + "/hook"})
	if err := n.Send(context.Background(), "Reminder: Dana - birthday", "Dana's birthday is in 3 days"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/hook" {
		t.Errorf("request path = %q, want %q", gotPath, "/hook")
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(gotBody), &envelope); err != nil {
		t.Fatalf("body not valid JSON: %v", err)
	}
	if envelope["title"] != "Reminder: Dana - birthday" {
		t.Errorf("envelope title = %v", envelope["title"])
	}
	if envelope["message"] != "Dana's birthday is in 3 days" {
		t.Errorf("envelope message = %v", envelope["message"])
	}
	if envelope["channel"] != "webhook" {
		t.Errorf("envelope channel = %v, want webhook", envelope["channel"])
	}
	sentAt, ok := envelope["sent_at"].(string)
	if !ok {
		t.Fatal("envelope sent_at missing or not a string")
	}
	if _, err := time.Parse(time.RFC3339, sentAt); err != nil {
		t.Errorf("sent_at %q is not RFC3339: %v", sentAt, err)
	}
}

func TestWebhookNotifier_Send_MultiURLFanOut(t *testing.T) {
	var mu sync.Mutex
	received := make([]string, 0, 2)

	handler := func(id string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			received = append(received, id)
			w.WriteHeader(http.StatusOK)
		}
	}
	srv1 := httptest.NewServer(handler("one"))
	defer srv1.Close()
	srv2 := httptest.NewServer(handler("two"))
	defer srv2.Close()

	n := NewWebhookNotifier(&config.Config{
		WebhookURL: srv1.URL + "/a, " + srv2.URL + "/b",
	})
	if err := n.Send(context.Background(), "t", "m"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("received %d requests, want 2 (received: %v)", len(received), received)
	}
	if received[0] != "one" || received[1] != "two" {
		t.Errorf("fan-out order = %v, want [one two]", received)
	}
}

func TestWebhookNotifier_Send_SignatureHeader(t *testing.T) {
	var gotSig, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Datey-Signature")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(&config.Config{
		WebhookURL:    srv.URL,
		WebhookSecret: "s3cret",
	})
	if err := n.Send(context.Background(), "t", "m"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	mac := hmac.New(sha256.New, []byte("s3cret"))
	_, _ = mac.Write([]byte(gotBody))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSig != want {
		t.Errorf("X-Datey-Signature = %q, want %q", gotSig, want)
	}
}

func TestWebhookNotifier_Send_NoSignatureWithoutSecret(t *testing.T) {
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Datey-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(&config.Config{WebhookURL: srv.URL})
	if err := n.Send(context.Background(), "t", "m"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotSig != "" {
		t.Errorf("X-Datey-Signature set without secret: %q", gotSig)
	}
}

func TestWebhookNotifier_Send_ErrorPropagation(t *testing.T) {
	t.Run("server error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		n := NewWebhookNotifier(&config.Config{WebhookURL: srv.URL})
		err := n.Send(context.Background(), "t", "m")
		if err == nil {
			t.Fatal("expected error for 500 status, got nil")
		}
		if !strings.Contains(err.Error(), "status 500") {
			t.Errorf("error = %q, want it to mention status 500", err)
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		n := NewWebhookNotifier(&config.Config{WebhookURL: "http://127.0.0.1:1"})
		if err := n.Send(context.Background(), "t", "m"); err == nil {
			t.Error("expected error for unreachable server, got nil")
		}
	})

	t.Run("one bad URL fails the channel", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		n := NewWebhookNotifier(&config.Config{WebhookURL: srv.URL + ", http://127.0.0.1:1"})
		if err := n.Send(context.Background(), "t", "m"); err == nil {
			t.Error("expected error when one of two URLs fails, got nil")
		}
	})
}

func TestWebhookNotifier_Send_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(&config.Config{WebhookURL: srv.URL})
	n.client = &http.Client{Timeout: 20 * time.Millisecond}

	start := time.Now()
	err := n.Send(context.Background(), "t", "m")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Errorf("Send() took %v, want it to fail quickly on timeout", elapsed)
	}
}
