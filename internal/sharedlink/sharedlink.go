// Package sharedlink issues and verifies stateless, signed, expiring tokens
// that grant read-only access to one person or one group.
//
// A token is self-contained; nothing is stored in the database:
//
//	token = base64url(payloadJSON) + "." + base64url(HMAC-SHA256(secret, base64url(payloadJSON)))
//
// payloadJSON = {"k":"person"|"group","id":<int>,"exp":<unix seconds>,"n":<nonce>}
//
// Because the payload is signed, a token can only be minted or extended by
// whoever holds the secret. Rotating that secret (the feed key in Datey)
// invalidates every outstanding token, which is the intended revocation
// mechanism.
package sharedlink

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	// KindPerson grants read-only access to one person's upcoming events.
	KindPerson = "person"
	// KindGroup grants read-only access to one group and its members.
	KindGroup = "group"

	// DefaultTTL is the lifetime used when no TTL (or a non-positive one) is
	// supplied.
	DefaultTTL = 30 * 24 * time.Hour
	// MinTTL is the shortest TTL Sign will issue.
	MinTTL = 24 * time.Hour
	// MaxTTL is the longest TTL Sign will issue.
	MaxTTL = 90 * 24 * time.Hour

	// ClockSkew is the allowance applied when checking expiry. A token that
	// expired less than ClockSkew ago is still accepted, so a small clock
	// difference between the issuer and a viewer does not break a link. There
	// is no tolerance in the other direction: an expiry far in the future
	// (beyond MaxTTL) is rejected because Sign never issues one.
	ClockSkew = 30 * time.Second

	// maxTokenLen caps the attacker-controlled input before any parsing.
	maxTokenLen = 4096
	// maxPayloadLen caps the decoded payload JSON.
	maxPayloadLen = 1024
	// nonceBytes is the entropy of the per-token nonce (12 base64url chars).
	nonceBytes = 9
)

// Typed token errors. Callers must not distinguish them in user-facing
// responses: every failure to verify a token is a 404.
var (
	ErrMalformed    = errors.New("sharedlink: malformed token")
	ErrBadSignature = errors.New("sharedlink: bad signature")
	ErrExpired      = errors.New("sharedlink: token expired")
	ErrUnknownKind  = errors.New("sharedlink: unknown kind")
	ErrNoSecret     = errors.New("sharedlink: signing secret is not configured")
)

// Payload is the verified content of a share token.
type Payload struct {
	// Kind is KindPerson or KindGroup.
	Kind string
	// ID is the person or group ID the token grants access to.
	ID int
	// Expires is the instant the token stops being accepted (plus ClockSkew).
	Expires time.Time
	// Nonce makes otherwise identical tokens distinct.
	Nonce string
}

// wirePayload is the exact JSON shape written into a token.
type wirePayload struct {
	Kind    string `json:"k"`
	ID      int    `json:"id"`
	Expires int64  `json:"exp"`
	Nonce   string `json:"n"`
}

// Signer signs and verifies share tokens with a shared secret.
type Signer struct {
	secret []byte
}

// NewSigner returns a Signer using secret. An empty secret makes Sign and
// Parse fail with ErrNoSecret rather than silently accepting forgeable tokens.
func NewSigner(secret []byte) *Signer {
	return &Signer{secret: secret}
}

// ClampTTL bounds ttl to [MinTTL, MaxTTL]; a non-positive TTL becomes
// DefaultTTL. One place so callers cannot drift from the documented policy.
func ClampTTL(ttl time.Duration) time.Duration {
	switch {
	case ttl <= 0:
		return DefaultTTL
	case ttl < MinTTL:
		return MinTTL
	case ttl > MaxTTL:
		return MaxTTL
	}
	return ttl
}

// Sign mints a token for kind/id valid for ttl (clamped by ClampTTL) from now.
func (s *Signer) Sign(kind string, id int, ttl time.Duration, now time.Time) (string, error) {
	if len(s.secret) == 0 {
		return "", ErrNoSecret
	}
	if kind != KindPerson && kind != KindGroup {
		return "", ErrUnknownKind
	}
	if id <= 0 {
		return "", ErrMalformed
	}

	nonce, err := newNonce()
	if err != nil {
		return "", err
	}

	raw, err := json.Marshal(wirePayload{
		Kind:    kind,
		ID:      id,
		Expires: now.Add(ClampTTL(ttl)).Unix(),
		Nonce:   nonce,
	})
	if err != nil {
		return "", err
	}

	body := base64.RawURLEncoding.EncodeToString(raw)
	return body + "." + base64.RawURLEncoding.EncodeToString(s.mac(body)), nil
}

// Parse verifies token and returns its payload. now is injected so callers and
// tests control the clock. Every failure returns one of the typed errors and
// no payload.
func (s *Signer) Parse(token string, now time.Time) (Payload, error) {
	if len(s.secret) == 0 {
		return Payload{}, ErrNoSecret
	}
	if token == "" || len(token) > maxTokenLen {
		return Payload{}, ErrMalformed
	}

	body, sigPart, ok := strings.Cut(token, ".")
	if !ok || body == "" || sigPart == "" || strings.Contains(sigPart, ".") {
		return Payload{}, ErrMalformed
	}

	given, err := base64.RawURLEncoding.DecodeString(sigPart)
	if err != nil || len(given) != sha256.Size {
		return Payload{}, ErrMalformed
	}
	// Signature first: only a token minted with our secret gets parsed.
	if !hmac.Equal(given, s.mac(body)) {
		return Payload{}, ErrBadSignature
	}

	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil || len(raw) == 0 || len(raw) > maxPayloadLen {
		return Payload{}, ErrMalformed
	}

	var w wirePayload
	// json.Unmarshal rejects trailing bytes after the object.
	if err := json.Unmarshal(raw, &w); err != nil {
		return Payload{}, ErrMalformed
	}

	if w.Kind != KindPerson && w.Kind != KindGroup {
		return Payload{}, ErrUnknownKind
	}
	if w.ID <= 0 || w.Expires <= 0 {
		return Payload{}, ErrMalformed
	}

	expires := time.Unix(w.Expires, 0)
	if now.Sub(expires) > ClockSkew {
		return Payload{}, ErrExpired
	}
	// Sign never issues an expiry further out than MaxTTL; anything beyond
	// that is signer misuse (or a hand-crafted token signed with a leaked key).
	if expires.After(now.Add(MaxTTL + ClockSkew)) {
		return Payload{}, ErrMalformed
	}

	return Payload{Kind: w.Kind, ID: w.ID, Expires: expires, Nonce: w.Nonce}, nil
}

// mac returns the raw HMAC-SHA256 of body under the signer's secret.
func (s *Signer) mac(body string) []byte {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(body))
	return m.Sum(nil)
}

// newNonce returns a fresh URL-safe random nonce.
func newNonce() (string, error) {
	b := make([]byte, nonceBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
