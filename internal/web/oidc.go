package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/session"
)

const (
	oidcStateCookie    = "oidc_state"
	oidcNonceCookie    = "oidc_nonce"
	oidcVerifierCookie = "oidc_verifier"
	oidcCookieMaxAge   = 600 // 10 minutes — just long enough to finish the IdP round-trip
	oidcAdminGroup     = "admins"
)

// oidcProvider caches the discovered provider (JWKS, endpoints) after the
// first successful discovery so logins don't re-fetch on every request.
var (
	oidcProviderMu sync.Mutex
	oidcProviders  = map[string]*oidc.Provider{}
)

// oidcClaims is the subset of ID token claims Datey uses.
type oidcClaims struct {
	Sub               string   `json:"sub"`
	Email             string   `json:"email"`
	EmailVerified     bool     `json:"email_verified"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
	Nonce             string   `json:"nonce"`
}

// oidcEnabled reports whether OIDC login is configured and switched on.
func (h *Handler) oidcEnabled() bool {
	return h.cfg.OIDCEnabled && h.cfg.OIDCIssuerURL != "" &&
		h.cfg.OIDCClientID != "" && h.cfg.OIDCClientSecret != ""
}

// oidcDiscover returns the cached OIDC provider, discovering it on first use.
func oidcDiscover(ctx context.Context, issuer string) (*oidc.Provider, error) {
	oidcProviderMu.Lock()
	defer oidcProviderMu.Unlock()
	if p, ok := oidcProviders[issuer]; ok {
		return p, nil
	}
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	oidcProviders[issuer] = p
	return p, nil
}

// oidcOAuthConfig builds the OAuth2 config for this request. The redirect URL
// comes from OIDC_REDIRECT_URL when set, otherwise from the incoming request
// (same pattern as the Google OAuth handlers).
func (h *Handler) oidcOAuthConfig(r *http.Request, provider *oidc.Provider) *oauth2.Config {
	redirectURL := h.cfg.OIDCRedirectURL
	if redirectURL == "" {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		redirectURL = scheme + "://" + r.Host + "/api/auth/oidc/callback"
	}
	return &oauth2.Config{
		ClientID:     h.cfg.OIDCClientID,
		ClientSecret: h.cfg.OIDCClientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       oidcParseScopes(h.cfg.OIDCScopes),
	}
}

// oidcParseScopes splits the OIDC_SCOPES env value (comma- or space-separated).
func oidcParseScopes(s string) []string {
	s = strings.ReplaceAll(s, ",", " ")
	if out := strings.Fields(s); len(out) > 0 {
		return out
	}
	return []string{"openid", "email", "profile", "groups"}
}

func oidcRandHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func oidcSecure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

func oidcSetCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   oidcSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   oidcCookieMaxAge,
	})
}

func oidcClearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1})
}

// oidcLogin starts the Authorization Code + PKCE flow.
func (h *Handler) oidcLogin(w http.ResponseWriter, r *http.Request) {
	if !h.oidcEnabled() {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	provider, err := oidcDiscover(ctx, h.cfg.OIDCIssuerURL)
	if err != nil {
		slog.Error("oidc: discovery failed", "error", err)
		http.Redirect(w, r, "/login?error=Single+sign-on+is+temporarily+unavailable", http.StatusSeeOther)
		return
	}
	state, err := oidcRandHex(16)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	nonce, err := oidcRandHex(16)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()
	oidcSetCookie(w, r, oidcStateCookie, state)
	oidcSetCookie(w, r, oidcNonceCookie, nonce)
	oidcSetCookie(w, r, oidcVerifierCookie, verifier)

	authURL := h.oidcOAuthConfig(r, provider).AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", oauth2.S256ChallengeFromVerifier(verifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("nonce", nonce),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// oidcCallback validates the IdP response, provisions/links the user, and
// issues the same session cookie as password login.
func (h *Handler) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if !h.oidcEnabled() {
		http.NotFound(w, r)
		return
	}
	fail := func(reason string) {
		h.auditRecord(r, "auth.login_failure", "oidc")
		slog.Warn("oidc: login rejected", "reason", reason)
		oidcClearCookie(w, oidcStateCookie)
		oidcClearCookie(w, oidcNonceCookie)
		oidcClearCookie(w, oidcVerifierCookie)
		http.Redirect(w, r, "/login?error=Single+sign-on+login+failed", http.StatusSeeOther)
	}

	stateCookie, err := r.Cookie(oidcStateCookie)
	if err != nil || stateCookie.Value == "" {
		fail("missing state cookie")
		return
	}
	nonceCookie, err := r.Cookie(oidcNonceCookie)
	if err != nil || nonceCookie.Value == "" {
		fail("missing nonce cookie")
		return
	}
	verifierCookie, err := r.Cookie(oidcVerifierCookie)
	if err != nil || verifierCookie.Value == "" {
		fail("missing verifier cookie")
		return
	}
	if q := r.URL.Query().Get("state"); q == "" || q != stateCookie.Value {
		fail("state mismatch")
		return
	}
	if q := r.URL.Query().Get("error"); q != "" {
		fail("idp error: " + q)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		fail("missing code")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	provider, err := oidcDiscover(ctx, h.cfg.OIDCIssuerURL)
	if err != nil {
		fail("discovery failed")
		return
	}
	oauthCfg := h.oidcOAuthConfig(r, provider)
	token, err := oauthCfg.Exchange(ctx, code, oauth2.VerifierOption(verifierCookie.Value))
	if err != nil {
		fail("code exchange failed")
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		fail("missing id_token")
		return
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: h.cfg.OIDCClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		fail("id_token verification failed")
		return
	}
	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		fail("claims parse failed")
		return
	}
	if claims.Nonce == "" || claims.Nonce != nonceCookie.Value {
		fail("nonce mismatch")
		return
	}
	if !claims.EmailVerified || claims.Email == "" {
		fail("unverified or missing email")
		return
	}
	if claims.Sub == "" {
		fail("missing sub")
		return
	}

	u, err := h.oidcFindOrProvision(r, &claims)
	if err != nil {
		slog.Error("oidc: provision failed", "error", err)
		fail("provisioning failed")
		return
	}

	oidcClearCookie(w, oidcStateCookie)
	oidcClearCookie(w, oidcNonceCookie)
	oidcClearCookie(w, oidcVerifierCookie)

	raw, err := h.sessions.Create(r.Context(), u.ID)
	if err != nil {
		slog.Error("oidc: create session", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	session.SetCookie(w, raw, oidcSecure(r))
	h.auditRecord(r, "auth.login_success", u.Username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// oidcFindOrProvision links by oidc_sub, then verified email, else creates.
// The admin flag syncs from the groups claim on every login; the very first
// user in a fresh DB becomes admin (same as /setup bootstrap).
func (h *Handler) oidcFindOrProvision(r *http.Request, claims *oidcClaims) (*ent.User, error) {
	ctx := r.Context()
	sub := h.cfg.OIDCIssuerURL + "|" + claims.Sub

	if u, err := h.users.GetByOIDCSub(ctx, sub); err == nil && u != nil {
		h.oidcSyncAdmin(ctx, u, claims.Groups)
		return h.users.GetByID(ctx, u.ID)
	}
	if u, err := h.users.GetByEmail(ctx, claims.Email); err == nil && u != nil {
		if err := h.users.LinkOIDCSub(ctx, u.ID, claims.Email, sub); err != nil {
			return nil, err
		}
		h.oidcSyncAdmin(ctx, u, claims.Groups)
		return h.users.GetByID(ctx, u.ID)
	}

	// First user in a fresh DB bootstraps as admin even without the group.
	exists, err := h.users.Exists(ctx)
	if err != nil {
		return nil, err
	}
	role := user.RoleUser
	if !exists || oidcIsAdmin(claims.Groups) {
		role = user.RoleAdmin
	}
	username := h.oidcUniqueUsername(ctx, claims)
	secret, err := oidcRandHex(32) // unusable password — OIDC users never use password login
	if err != nil {
		return nil, err
	}
	u, err := h.users.CreateOIDC(ctx, username, claims.Email, sub, "oidc$"+secret, role)
	if err != nil {
		return nil, fmt.Errorf("create oidc user: %w", err)
	}
	h.auditRecord(r, "user.create", username)
	return u, nil
}

// oidcSyncAdmin aligns the admin flag with the groups claim.
func (h *Handler) oidcSyncAdmin(ctx context.Context, u *ent.User, groups []string) {
	want := user.RoleUser
	if oidcIsAdmin(groups) {
		want = user.RoleAdmin
	}
	if u.Role != want {
		if err := h.users.SetRole(ctx, u.ID, want); err != nil {
			slog.Warn("oidc: sync admin flag", "error", err, "user", u.Username)
		}
	}
}

// oidcUniqueUsername derives a username from the claims, appending a suffix
// until it is unique.
func (h *Handler) oidcUniqueUsername(ctx context.Context, claims *oidcClaims) string {
	base := strings.TrimSpace(claims.PreferredUsername)
	if base == "" {
		base = strings.TrimSpace(claims.Name)
	}
	if base == "" && strings.Contains(claims.Email, "@") {
		base = strings.SplitN(claims.Email, "@", 2)[0]
	}
	if base == "" {
		base = "user"
	}
	base = strings.ToLower(strings.ReplaceAll(base, " ", "_"))
	candidate := base
	for i := 2; ; i++ {
		if _, err := h.users.GetByUsername(ctx, candidate); err != nil {
			return candidate // not taken
		}
		candidate = fmt.Sprintf("%s%d", base, i)
	}
}

func (h *Handler) oidcLogout(w http.ResponseWriter, r *http.Request) {
	actor := ""
	if u := UserFromContext(r.Context()); u != nil {
		actor = u.Username
	}
	if token, err := session.ReadCookie(r); err == nil && token != "" {
		if err := h.sessions.Delete(r.Context(), token); err != nil {
			slog.Warn("oidc logout: delete session", "error", err)
		}
	}
	h.auditRecord(r, "auth.logout", actor)
	session.ClearCookie(w)

	if h.oidcEnabled() {
		dest := strings.TrimSuffix(h.cfg.OIDCIssuerURL, "/") + "/logout"
		if h.cfg.AppURL != "" {
			dest += "?post_logout_redirect_uri=" + url.QueryEscape(h.cfg.AppURL)
		}
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// oidcIsAdmin reports whether the groups claim grants admin.
func oidcIsAdmin(groups []string) bool {
	for _, g := range groups {
		if g == oidcAdminGroup {
			return true
		}
	}
	return false
}
