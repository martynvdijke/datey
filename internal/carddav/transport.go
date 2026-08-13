// Package carddav implements a minimal CardDAV client and a two-way sync
// engine. The WebDAV/CardDAV protocol surface used here (PROPFIND,
// sync-collection REPORT, GET/PUT/DELETE) is small enough to hand-roll over
// net/http; the only external dependency remains go-vcard for vCard parsing.
package carddav

import (
	"encoding/base64"
	"net/http"
)

// Transport performs HTTP requests. It is the seam where authentication is
// applied: BasicAuthTransport covers Nextcloud/Baikal/iCloud-style servers,
// and OAuth2Transport (a stub) will cover Google in a later iteration.
type Transport interface {
	Do(req *http.Request) (*http.Response, error)
}

// BasicAuthTransport attaches an Authorization: Basic header to every request.
type BasicAuthTransport struct {
	Username string
	Password string
	Base     *http.Client
}

func (t *BasicAuthTransport) Do(req *http.Request) (*http.Response, error) {
	if t.Username != "" || t.Password != "" {
		token := base64.StdEncoding.EncodeToString([]byte(t.Username + ":" + t.Password))
		req.Header.Set("Authorization", "Basic "+token)
	}
	client := t.Base
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

// OAuth2Transport is a stub for Google CardDAV. The go.mod intentionally does
// not carry golang.org/x/oauth2 yet; this type documents the seam and fails
// loudly if configured before the OAuth flow is implemented.
type OAuth2Transport struct {
	// Token is the bearer token to attach. Populated by a future OAuth flow.
	Token string
	Base  *http.Client
}

func (t *OAuth2Transport) Do(req *http.Request) (*http.Response, error) {
	if t.Token == "" {
		return nil, &AuthError{"OAuth2 carddav transport is not implemented; use basic auth or configure an OAuth token"}
	}
	req.Header.Set("Authorization", "Bearer "+t.Token)
	client := t.Base
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

// AuthError reports an authentication/configuration problem with the
// CardDAV transport.
type AuthError struct {
	msg string
}

func (e *AuthError) Error() string { return e.msg }
