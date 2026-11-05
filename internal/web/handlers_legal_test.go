package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/account"
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
				"Version 1.0",
				"Standard Contractual Clauses",
				"supervisory authority",
				"data protection authority",
			},
		},
		{
			name: "terms",
			path: "/terms",
			fn:   h.termsPage,
			want: []string{"Terms of use", "/privacy", "Data protection and consumer rights"},
		},
		{
			name: "account delete",
			path: "/account/delete",
			fn:   h.accountDeletePage,
			want: []string{"Delete your account", "Delete a person who has no account", "Sign in", "Email an account deletion request", "mailto:privacy@example.test"},
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
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := w.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", got)
	}
}

// assertDeletionAudit checks that after a self-deletion the audit log keeps the
// action but no longer contains the deleted identity.
func assertDeletionAudit(t *testing.T, h *Handler, userID int, username, email string) {
	t.Helper()
	entries, err := h.client.AuditEntry.Query().All(context.Background())
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	found := false
	for _, e := range entries {
		joined := strings.ToLower(e.ActorUsername + "\x00" + e.Target)
		if strings.Contains(joined, strings.ToLower(username)) {
			t.Fatalf("audit entry retains username %q: actor=%q target=%q", username, e.ActorUsername, e.Target)
		}
		if email != "" && strings.Contains(joined, strings.ToLower(email)) {
			t.Fatalf("audit entry retains email %q: actor=%q target=%q", email, e.ActorUsername, e.Target)
		}
		if e.Action == "account.delete_self" && e.ActorUsername == account.DeletedActor && e.Target == fmt.Sprintf("user:%d", userID) {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing pseudonymised account.delete_self entry actor=%q target=%q: %#v", account.DeletedActor, fmt.Sprintf("user:%d", userID), entries)
	}
}

func TestWebSelfDeleteAuditIsPseudonymised(t *testing.T) {
	h, _ := newAuditTestHandler(t)
	u := seedRegularUser(t, h, "alice")
	u = h.client.User.UpdateOneID(u.ID).SetEmail("alice@example.test").SaveX(context.Background())

	req := httptest.NewRequest("POST", "/settings/account/delete", strings.NewReader("confirm=alice"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, u))
	req = req.WithContext(context.WithValue(req.Context(), "user", u)) //nolint:staticcheck
	w := httptest.NewRecorder()
	h.accountDeleteSelf(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if h.client.User.Query().Where(user.UsernameEQ("alice")).ExistX(context.Background()) {
		t.Fatal("account still exists")
	}
	assertDeletionAudit(t, h, u.ID, "alice", "alice@example.test")
}

func TestAPISelfDeleteAuditIsPseudonymised(t *testing.T) {
	h, _ := newAuditTestHandler(t)
	u := seedRegularUser(t, h, "alice")
	u = h.client.User.UpdateOneID(u.ID).SetEmail("alice@example.test").SaveX(context.Background())

	req := httptest.NewRequest("DELETE", "/api/v1/me", strings.NewReader(`{"confirm":"alice"}`)).
		WithContext(context.WithValue(context.Background(), userContextKey, u))
	req = req.WithContext(context.WithValue(req.Context(), "user", u)) //nolint:staticcheck
	w := httptest.NewRecorder()
	h.apiDeleteMe(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	assertDeletionAudit(t, h, u.ID, "alice", "alice@example.test")
}

func TestAccountDeletePageSignedInOffersExport(t *testing.T) {
	h := newTestWebHandler(t)
	u := h.client.User.Create().
		SetUsername("alice").
		SetPasswordHash("x").
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		SaveX(context.Background())

	req := httptest.NewRequest("GET", "/account/delete", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, u))
	w := httptest.NewRecorder()
	h.accountDeletePage(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`href="/api/v1/me/export"`,
		"Download my data (JSON)",
		"right to data portability",
		"gift ideas",
		"excluded",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in signed-in deletion page", want)
		}
	}
}

func TestSettingsPageOffersExport(t *testing.T) {
	h := newTestWebHandler(t)
	u := h.client.User.Create().
		SetUsername("alice").
		SetPasswordHash("x").
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		SaveX(context.Background())

	req := httptest.NewRequest("GET", "/settings", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, u))
	w := httptest.NewRecorder()
	h.settings(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`href="/api/v1/me/export"`, "Download my data (JSON)", "right to data portability"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in settings page", want)
		}
	}
}

func TestAccountDeletePageSignedOutContact(t *testing.T) {
	t.Run("with contact email", func(t *testing.T) {
		h := newTestWebHandler(t)
		h.cfg.LegalContactEmail = "privacy@example.test"
		w := httptest.NewRecorder()
		h.accountDeletePage(w, httptest.NewRequest("GET", "/account/delete", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		for _, want := range []string{
			"mailto:privacy@example.test",
			"subject=Account%20deletion%20request%20%E2%80%94%20Datey",
			"My%20username%20is%3A",
			"Email an account deletion request",
			"privacy@example.test",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("missing %q in signed-out deletion page", want)
			}
		}
	})

	t.Run("without contact email", func(t *testing.T) {
		h := newTestWebHandler(t)
		h.cfg.LegalContactEmail = ""
		h.cfg.LegalControllerName = "Example Family Cooperative"
		w := httptest.NewRecorder()
		h.accountDeletePage(w, httptest.NewRequest("GET", "/account/delete", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		for _, want := range []string{
			"Example Family Cooperative",
			"Contact the administrator",
			"no self-service deletion route",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("missing %q in fallback deletion page", want)
			}
		}
		if strings.Contains(body, "mailto:") {
			t.Fatal("fallback page should not render a mailto contact")
		}
	})
}
