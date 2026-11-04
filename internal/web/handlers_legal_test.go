package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent/user"
)

func TestPublicLegalPages(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.LegalControllerName = "Example Family Cooperative"
	h.cfg.LegalContactEmail = "privacy@example.test"
	h.cfg.LegalHostingRegion = "the Netherlands"
	h.cfg.GoogleContactsEnabled = true
	h.cfg.UmamiURL = "https://umami.example.test"

	tests := []struct {
		name string
		path string
		fn   func(http.ResponseWriter, *http.Request)
		want []string
	}{
		{
			name: "privacy",
			path: "/privacy",
			fn:   h.privacyPage,
			want: []string{
				"Privacy policy",
				"Example Family Cooperative",
				"privacy@example.test",
				"the Netherlands",
				"Google API Services User Data Policy (Limited Use)",
				"https://umami.example.test",
				"/account/delete",
			},
		},
		{
			name: "terms",
			path: "/terms",
			fn:   h.termsPage,
			want: []string{"Terms of use", "/privacy"},
		},
		{
			name: "account delete",
			path: "/account/delete",
			fn:   h.accountDeletePage,
			want: []string{"Delete your account", "Delete a person who has no account", "Sign in"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()
			tt.fn(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			body := w.Body.String()
			for _, want := range tt.want {
				if !strings.Contains(body, want) {
					t.Fatalf("missing %q in page", want)
				}
			}
		})
	}
}

func TestAPIDeleteMeRequiresAuth(t *testing.T) {
	h := newTestWebHandler(t)
	w := httptest.NewRecorder()
	h.apiDeleteMe(w, httptest.NewRequest("DELETE", "/api/v1/me", strings.NewReader(`{"confirm":"DELETE"}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestAPIDeleteMeRequiresConfirmation(t *testing.T) {
	h := newTestWebHandler(t)
	u := h.client.User.Create().
		SetUsername("alice").
		SetPasswordHash("x").
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		SaveX(context.Background())

	req := httptest.NewRequest("DELETE", "/api/v1/me", strings.NewReader(`{"confirm":"wrong"}`)).
		WithContext(context.WithValue(context.Background(), userContextKey, u))
	w := httptest.NewRecorder()
	h.apiDeleteMe(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if !h.client.User.Query().Where(user.UsernameEQ("alice")).ExistX(context.Background()) {
		t.Fatal("account deleted despite bad confirmation")
	}
}

func TestAPIDeleteMeDeletesAccount(t *testing.T) {
	h := newTestWebHandler(t)
	u := h.client.User.Create().
		SetUsername("alice").
		SetPasswordHash("x").
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		SaveX(context.Background())

	req := httptest.NewRequest("DELETE", "/api/v1/me", strings.NewReader(`{"confirm":"alice"}`)).
		WithContext(context.WithValue(context.Background(), userContextKey, u))
	w := httptest.NewRecorder()
	h.apiDeleteMe(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if h.client.User.Query().Where(user.UsernameEQ("alice")).ExistX(context.Background()) {
		t.Fatal("account still exists")
	}
}

func TestAPIDeleteMeRefusesLastAdmin(t *testing.T) {
	h := newTestWebHandler(t)
	u := h.client.User.Create().
		SetUsername("admin").
		SetPasswordHash("x").
		SetRole(user.RoleAdmin).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		SaveX(context.Background())

	req := httptest.NewRequest("DELETE", "/api/v1/me", strings.NewReader(`{"confirm":"DELETE"}`)).
		WithContext(context.WithValue(context.Background(), userContextKey, u))
	w := httptest.NewRecorder()
	h.apiDeleteMe(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "last_admin" {
		t.Fatalf("error %q", body["error"])
	}
	if !h.client.User.Query().Where(user.UsernameEQ("admin")).ExistX(context.Background()) {
		t.Fatal("last admin was deleted")
	}
}

func TestAPIExportMe(t *testing.T) {
	h := newTestWebHandler(t)
	u := h.client.User.Create().
		SetUsername("alice").
		SetPasswordHash("x").
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		SaveX(context.Background())

	req := httptest.NewRequest("GET", "/api/v1/me/export", nil).
		WithContext(context.WithValue(context.Background(), userContextKey, u))
	w := httptest.NewRecorder()
	h.apiExportMe(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"username":"alice"`) {
		t.Fatalf("missing account in export: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "password_hash") {
		t.Fatal("export leaked a password hash")
	}
}
