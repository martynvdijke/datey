package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datey/datey/ent/user"
)

// Disabled OIDC must 404 on login/callback (rollback path: OIDC_ENABLED=false).
func TestOIDCDisabledRoutesNotFound(t *testing.T) {
	h := newTestWebHandler(t)
	for _, path := range []string{"/api/auth/oidc/login", "/api/auth/oidc/callback"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		if path == "/api/auth/oidc/login" {
			h.oidcLogin(w, req)
		} else {
			h.oidcCallback(w, req)
		}
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", path, w.Code)
		}
	}
}

func TestOIDCParseScopes(t *testing.T) {
	if got := oidcParseScopes(""); len(got) != 4 || got[0] != "openid" {
		t.Fatalf("default scopes = %v", got)
	}
	if got := oidcParseScopes("openid,email,profile"); len(got) != 3 || got[1] != "email" {
		t.Fatalf("csv scopes = %v", got)
	}
}

func TestOIDCIsAdmin(t *testing.T) {
	if !oidcIsAdmin([]string{"users", "admins"}) {
		t.Fatal("groups with admins should be admin")
	}
	if oidcIsAdmin([]string{"users"}) {
		t.Fatal("groups without admins should not be admin")
	}
}

func TestOIDCUniqueUsername_AppendsSuffix(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := t.Context()
	if _, err := h.users.Create(ctx, "alice", "x", user.RoleUser); err != nil {
		t.Fatal(err)
	}
	got := h.oidcUniqueUsername(ctx, &oidcClaims{PreferredUsername: "alice"})
	if got != "alice2" {
		t.Fatalf("got %q, want alice2", got)
	}
	got = h.oidcUniqueUsername(ctx, &oidcClaims{Email: "bob@example.com"})
	if got != "bob" {
		t.Fatalf("got %q, want bob", got)
	}
}
