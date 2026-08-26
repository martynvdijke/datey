package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/datey/datey/ent"
	entuser "github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/repository"
)

// setupSecureRouter mirrors the production middleware chain
// (BearerAuth -> CSRF -> Auth) over a representative route subset:
// public TRMNL reads plus authenticated mutations and token management.
func setupSecureRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Use(h.BearerAuth)
	r.Use(h.CSRF)

	// Public reads — must stay accessible without any credentials.
	r.Get("/api/trmnl/stats", h.trmnlStats)
	r.Get("/api/trmnl/birthdays", h.trmnlBirthdays)

	r.Group(func(r chi.Router) {
		r.Use(h.Auth)
		r.Post("/people/new", h.createPerson)
		r.Get("/settings/api-tokens", h.apiTokensPage)
		r.Post("/settings/api-tokens", h.apiTokenCreate)
		r.Post("/settings/api-tokens/{id}/revoke", h.apiTokenRevoke)
		r.Post("/settings/api-tokens/{id}/rotate", h.apiTokenRotate)
	})
	return r
}

func createUserWithSession(t *testing.T, h *Handler, username string) (*ent.User, string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	u, err := h.users.Create(context.Background(), username, string(hash), entuser.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := h.sessions.Create(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return u, raw
}

// createSessionOnly creates a user and returns only their session cookie value.
func createSessionOnly(t *testing.T, h *Handler, username string) string {
	t.Helper()
	_, sess := createUserWithSession(t, h, username)
	return sess
}

var csrfFieldRe = regexp.MustCompile(`name="csrf_token" value="([0-9a-f]+)"`)

// fetchCSRF performs a GET of the token page to obtain the double-submit pair
// (cookie + form token) needed for session-authenticated POSTs.
func fetchCSRF(t *testing.T, r chi.Router, sessionRaw string) (*http.Cookie, string) {
	t.Helper()
	req := httptest.NewRequest("GET", "/settings/api-tokens", nil)
	if sessionRaw != "" {
		req.AddCookie(&http.Cookie{Name: "session", Value: sessionRaw})
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /settings/api-tokens = %d, want 200", w.Code)
	}
	m := csrfFieldRe.FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatal("csrf_token field missing from API tokens page")
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "csrf_token" && c.Value != "" {
			return c, m[1]
		}
	}
	t.Fatal("csrf_token cookie not set")
	return nil, ""
}

func postSecureForm(r chi.Router, path string, headers map[string]string, cookies []*http.Cookie, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func getReq(r chi.Router, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// --- Public reads (spec scenario: unauthenticated TRMNL access stays open) ---

func TestPublicTRMNLReads_RemainAccessible(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupSecureRouter(h)

	for _, path := range []string{"/api/trmnl/stats", "/api/trmnl/birthdays"} {
		w := getReq(r, path, nil)
		if w.Code != http.StatusOK {
			t.Errorf("%s without credentials = %d, want 200", path, w.Code)
		}
	}
}

// --- Anonymous mutations are rejected and change nothing ---

func TestAnonymousMutation_Rejected_ResourceUnchanged(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupSecureRouter(h)

	form := url.Values{"name": {"Sneaky Person"}}
	w := postSecureForm(r, "/people/new", nil, nil, form)

	// Production middleware order runs CSRF before Auth, so an anonymous
	// browser-style write fails CSRF validation (403). Either way the write
	// must be rejected and leave data untouched.
	if w.Code != http.StatusForbidden && w.Code != http.StatusSeeOther {
		t.Fatalf("anonymous POST /people/new = %d, want 403/303", w.Code)
	}
	n, err := h.client.Person.Query().Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("person count after anonymous write = %d, want 0", n)
	}
}

// --- Bearer token grants user identity for mutations without CSRF ---

func TestBearerToken_MutationSucceeds(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupSecureRouter(h)
	u, _ := createUserWithSession(t, h, "tokenuser")

	raw, _, err := h.apiTokens.Create(context.Background(), u.ID, "scripts", nil)
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{"name": {"Bearer Person"}}
	w := postSecureForm(r, "/people/new", map[string]string{"Authorization": "Bearer " + raw}, nil, form)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("bearer POST /people/new = %d, want 303", w.Code)
	}
	n, err := h.client.Person.Query().Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("person count after bearer write = %d, want 1", n)
	}
}

func TestBearerToken_MalformedRejected(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupSecureRouter(h)
	createUserWithSession(t, h, "malformed")

	cases := map[string]string{
		"wrong scheme": "Basic dXNlcjpwYXNz",
		"empty secret": "Bearer ",
		"unknown":      "Bearer deadbeef",
	}
	for name, authz := range cases {
		w := postSecureForm(r, "/people/new", map[string]string{"Authorization": authz}, nil,
			url.Values{"name": {"X"}})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: POST = %d, want 401", name, w.Code)
		}
	}
}

func TestBearerToken_ExpiredRejected(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupSecureRouter(h)
	u, _ := createUserWithSession(t, h, "expired")

	past := time.Now().Add(-time.Hour)
	raw, _, err := h.apiTokens.Create(context.Background(), u.ID, "old", &past)
	if err != nil {
		t.Fatal(err)
	}

	w := postSecureForm(r, "/people/new", map[string]string{"Authorization": "Bearer " + raw}, nil,
		url.Values{"name": {"X"}})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expired token POST = %d, want 401", w.Code)
	}
}

func TestBearerToken_RevokedRejected(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupSecureRouter(h)
	u, _ := createUserWithSession(t, h, "revoked")

	raw, tok, err := h.apiTokens.Create(context.Background(), u.ID, "doomed", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.apiTokens.Revoke(context.Background(), tok.ID, u.ID); err != nil {
		t.Fatal(err)
	}

	w := postSecureForm(r, "/people/new", map[string]string{"Authorization": "Bearer " + raw}, nil,
		url.Values{"name": {"X"}})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("revoked token POST = %d, want 401", w.Code)
	}
}

// --- Session flow keeps CSRF protection ---

func TestSessionMutation_WithoutCSRF_Rejected(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupSecureRouter(h)
	_, sess := createUserWithSession(t, h, "browser")

	w := postSecureForm(r, "/people/new", nil,
		[]*http.Cookie{{Name: "session", Value: sess}},
		url.Values{"name": {"X"}})
	if w.Code != http.StatusForbidden {
		t.Fatalf("session POST without CSRF = %d, want 403", w.Code)
	}
}

// --- Token lifecycle: one-time secret, metadata-only listing, rotate, revoke ---

func TestApiTokenLifecycle_CreateListRevokeRotate(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupSecureRouter(h)
	_, sess := createUserWithSession(t, h, "owner")

	csrfCookie, csrfTok := fetchCSRF(t, r, sess)
	cookies := []*http.Cookie{
		{Name: "session", Value: sess},
		csrfCookie,
	}
	csrfForm := url.Values{"csrf_token": {csrfTok}}

	// Create via UI: secret is displayed exactly once.
	form := url.Values{"name": {"ci-job"}, "expiry_days": {"30"}, "csrf_token": {csrfTok}}
	w := postSecureForm(r, "/settings/api-tokens", nil, cookies, form)
	if w.Code != http.StatusOK {
		t.Fatalf("create token = %d, want 200", w.Code)
	}
	body := w.Body.String()
	m := regexp.MustCompile(`value="([0-9a-f]{64})"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("raw token secret not shown after creation")
	}
	raw := m[1]

	// Listing afterwards shows metadata but never the secret or its hash.
	hash := repository.HashToken(raw)
	listReq := httptest.NewRequest("GET", "/settings/api-tokens", nil)
	listReq.AddCookie(&http.Cookie{Name: "session", Value: sess})
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, listReq)
	listBody := w2.Body.String()
	if strings.Contains(listBody, raw) {
		t.Error("listing leaks raw token secret")
	}
	if strings.Contains(listBody, hash) {
		t.Error("listing leaks token hash")
	}
	if !strings.Contains(listBody, "ci-job") {
		t.Error("listing does not show token name")
	}

	// The created token authenticates a mutation.
	w3 := postSecureForm(r, "/people/new", map[string]string{"Authorization": "Bearer " + raw}, nil,
		url.Values{"name": {"CI"}})
	if w3.Code != http.StatusSeeOther {
		t.Errorf("new token mutation = %d, want 303", w3.Code)
	}

	// Rotate: old secret dies immediately, new one is shown once.
	w4 := postSecureForm(r, "/settings/api-tokens/1/rotate", nil, cookies, csrfForm)
	if w4.Code != http.StatusOK {
		t.Fatalf("rotate = %d, want 200", w4.Code)
	}
	m4 := regexp.MustCompile(`value="([0-9a-f]{64})"`).FindStringSubmatch(w4.Body.String())
	if m4 == nil || m4[1] == raw {
		t.Fatal("rotate did not surface a fresh secret")
	}
	newRaw := m4[1]

	if w := postSecureForm(r, "/people/new", map[string]string{"Authorization": "Bearer " + raw}, nil,
		url.Values{"name": {"X"}}); w.Code != http.StatusUnauthorized {
		t.Errorf("old secret after rotate = %d, want 401", w.Code)
	}
	if w := postSecureForm(r, "/people/new", map[string]string{"Authorization": "Bearer " + newRaw}, nil,
		url.Values{"name": {"Y"}}); w.Code != http.StatusSeeOther {
		t.Errorf("rotated secret mutation = %d, want 303", w.Code)
	}

	// Revoke: token stops working but stays listed as revoked.
	w5 := postSecureForm(r, "/settings/api-tokens/1/revoke", nil, cookies, csrfForm)
	if w5.Code != http.StatusSeeOther {
		t.Fatalf("revoke = %d, want 303", w5.Code)
	}
	if w := postSecureForm(r, "/people/new", map[string]string{"Authorization": "Bearer " + newRaw}, nil,
		url.Values{"name": {"Z"}}); w.Code != http.StatusUnauthorized {
		t.Errorf("revoked token mutation = %d, want 401", w.Code)
	}
	w6req := httptest.NewRequest("GET", "/settings/api-tokens", nil)
	w6req.AddCookie(&http.Cookie{Name: "session", Value: sess})
	w6 := httptest.NewRecorder()
	r.ServeHTTP(w6, w6req)
	if !strings.Contains(w6.Body.String(), "Revoked") {
		t.Error("revoked token not flagged in listing")
	}
}

// --- Owner-only enforcement: foreign tokens answer 404 without state leaks ---

func TestApiTokenOwnership_ForeignOperationsRejected(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupSecureRouter(h)

	attackerSess := createSessionOnly(t, h, "attacker")
	victim, _ := createUserWithSession(t, h, "victim")

	raw, tok, err := h.apiTokens.Create(context.Background(), victim.ID, "victim-token", nil)
	if err != nil {
		t.Fatal(err)
	}

	csrfCookie, csrfTok := fetchCSRF(t, r, attackerSess)
	cookies := []*http.Cookie{
		{Name: "session", Value: attackerSess},
		csrfCookie,
	}
	csrfForm := url.Values{"csrf_token": {csrfTok}}

	if w := postSecureForm(r, "/settings/api-tokens/"+itoa(tok.ID)+"/revoke", nil, cookies, csrfForm); w.Code != http.StatusNotFound {
		t.Errorf("foreign revoke = %d, want 404", w.Code)
	}
	if w := postSecureForm(r, "/settings/api-tokens/"+itoa(tok.ID)+"/rotate", nil, cookies, csrfForm); w.Code != http.StatusNotFound {
		t.Errorf("foreign rotate = %d, want 404", w.Code)
	}

	// Victim's token still works untouched.
	if w := postSecureForm(r, "/people/new", map[string]string{"Authorization": "Bearer " + raw}, nil,
		url.Values{"name": {"V"}}); w.Code != http.StatusSeeOther {
		t.Errorf("victim token after foreign attempts = %d, want 303", w.Code)
	}

	// Sanity: owner can still revoke their own token.
	if err := h.apiTokens.Revoke(context.Background(), tok.ID, victim.ID); err != nil {
		t.Errorf("owner revoke failed: %v", err)
	}
}
