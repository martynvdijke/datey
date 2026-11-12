// Package reminderaction issues and verifies stateless, signed tokens that let
// a reminder recipient act on one occurrence straight from a notification —
// acknowledge it or snooze it for a few days — without signing in.
//
// A token is self-contained; nothing is stored to mint it:
//
//	token = base64url(payloadJSON) + "." + base64url(HMAC-SHA256(secret, base64url(payloadJSON)))
//
// payloadJSON = {"e":<eventID>,"o":"YYYY-MM-DD","u":<userID>,"a":"ack"|"snooze","x":<unix expiry>}
//
// Because the payload is signed, only the holder of the secret can mint a
// token, and a token can only grant an action on the exact occurrence it names.
// Rotating the secret (a feed key in Datey) invalidates every outstanding
// action link, which is the intended revocation mechanism.
package reminderaction

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	// ActionAck marks one occurrence as handled: no further reminders.
	ActionAck = "ack"
	// ActionSnooze postpones reminders for one occurrence for a few days.
	ActionSnooze = "snooze"

	// DefaultTTL is the lifetime used when no TTL (or a non-positive one) is
	// supplied. It is long on purpose: an action email commonly sits in an
	// inbox for weeks before it is used.
	DefaultTTL = 60 * 24 * time.Hour
	// MaxTTL is the longest TTL Mint issues.
	MaxTTL = 180 * 24 * time.Hour

	// ClockSkew is the allowance applied when checking expiry, so a small
	// clock difference between server and recipient does not reject a token.
	ClockSkew = 5 * time.Minute

	// dateLayout is the day-precision form an occurrence is carried in.
	dateLayout = "2006-01-02"

	// maxTokenLen caps the attacker-controlled input before any parsing.
	maxTokenLen = 4096
	// maxPayloadLen caps the decoded payload JSON.
	maxPayloadLen = 512
)

// Typed token errors. Callers must not distinguish them in user-facing
// responses: every failure to verify a token is a 404.
var (
	ErrMalformed    = errors.New("reminderaction: malformed token")
	ErrBadSignature = errors.New("reminderaction: bad signature")
	ErrExpired      = errors.New("reminderaction: token expired")
	ErrUnknown      = errors.New("reminderaction: unknown action")
	ErrNoSecret     = errors.New("reminderaction: signing secret is not configured")
)

// Action is the verified content of an action token.
type Action struct {
	eventID  int
	occasion time.Time
	userID   int
	kind     string
	expires  time.Time
}

// EventID is the event the action applies to.
func (a Action) EventID() int { return a.eventID }

// Occasion is the occurrence date (midnight UTC, day precision) the action
// applies to.
func (a Action) Occasion() time.Time { return a.occasion }

// UserID is the recipient the action applies to; 0 is the global sentinel
// used by notifications that are not addressed to a single user.
func (a Action) UserID() int { return a.userID }

// Kind is ActionAck or ActionSnooze.
func (a Action) Kind() string { return a.kind }

// Expires is the instant after which the token is rejected.
func (a Action) Expires() time.Time { return a.expires }

// IsAck reports whether the token grants an acknowledgement.
func (a Action) IsAck() bool { return a.kind == ActionAck }

// IsSnooze reports whether the token grants a snooze.
func (a Action) IsSnooze() bool { return a.kind == ActionSnooze }

// wirePayload is the exact JSON shape written into a token.
type wirePayload struct {
	EventID  int    `json:"e"`
	Occasion string `json:"o"`
	UserID   int    `json:"u"`
	Action   string `json:"a"`
	Expires  int64  `json:"x"`
}

// ClampTTL bounds ttl to [1s, MaxTTL]; a non-positive TTL becomes DefaultTTL.
func ClampTTL(ttl time.Duration) time.Duration {
	switch {
	case ttl <= 0:
		return DefaultTTL
	case ttl > MaxTTL:
		return MaxTTL
	}
	return ttl
}

// Mint returns a signed action token for one occurrence. occasion is reduced
// to its calendar day (UTC) because an action always applies to a whole
// occurrence, never to an instant. A nil or empty secret, an unknown action or
// a non-positive event ID is an error rather than a forgeable token.
func Mint(secret []byte, eventID int, occasion time.Time, userID int, action string, ttl time.Duration) (string, error) {
	if len(secret) == 0 {
		return "", ErrNoSecret
	}
	if action != ActionAck && action != ActionSnooze {
		return "", ErrUnknown
	}
	if eventID <= 0 || userID < 0 {
		return "", ErrMalformed
	}

	raw, err := json.Marshal(wirePayload{
		EventID:  eventID,
		Occasion: occasion.UTC().Format(dateLayout),
		UserID:   userID,
		Action:   action,
		Expires:  time.Now().Add(ClampTTL(ttl)).Unix(),
	})
	if err != nil {
		return "", err
	}

	body := base64.RawURLEncoding.EncodeToString(raw)
	return body + "." + base64.RawURLEncoding.EncodeToString(mac(secret, body)), nil
}

// Parse verifies token and returns its action, using the current time.
func Parse(secret []byte, token string) (Action, error) {
	return ParseAt(secret, token, time.Now())
}

// ParseAt is Parse with an injected clock, so tests control expiry. Every
// failure returns one of the typed errors and no action.
func ParseAt(secret []byte, token string, now time.Time) (Action, error) {
	if len(secret) == 0 {
		return Action{}, ErrNoSecret
	}
	if token == "" || len(token) > maxTokenLen {
		return Action{}, ErrMalformed
	}

	body, sigPart, ok := strings.Cut(token, ".")
	if !ok || body == "" || sigPart == "" || strings.Contains(sigPart, ".") {
		return Action{}, ErrMalformed
	}

	given, err := base64.RawURLEncoding.DecodeString(sigPart)
	if err != nil || len(given) != sha256.Size {
		return Action{}, ErrMalformed
	}
	// Signature first: only a token minted with our secret gets parsed.
	if !hmac.Equal(given, mac(secret, body)) {
		return Action{}, ErrBadSignature
	}

	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil || len(raw) == 0 || len(raw) > maxPayloadLen {
		return Action{}, ErrMalformed
	}

	var w wirePayload
	if err := json.Unmarshal(raw, &w); err != nil {
		return Action{}, ErrMalformed
	}
	if w.Action != ActionAck && w.Action != ActionSnooze {
		return Action{}, ErrUnknown
	}
	if w.EventID <= 0 || w.UserID < 0 || w.Expires <= 0 {
		return Action{}, ErrMalformed
	}

	occasion, err := time.Parse(dateLayout, w.Occasion)
	if err != nil {
		return Action{}, ErrMalformed
	}

	expires := time.Unix(w.Expires, 0)
	if now.Sub(expires) > ClockSkew {
		return Action{}, ErrExpired
	}
	// Mint never issues an expiry further out than MaxTTL; anything beyond that
	// is signer misuse (or a hand-crafted token signed with a leaked key).
	if expires.After(now.Add(MaxTTL + ClockSkew)) {
		return Action{}, ErrMalformed
	}

	return Action{
		eventID:  w.EventID,
		occasion: occasion,
		userID:   w.UserID,
		kind:     w.Action,
		expires:  expires,
	}, nil
}

// URL returns the absolute action URL for token under base (the configured
// APP_URL). An empty base yields a path-only URL, which is still valid for a
// same-origin link.
func URL(base, token string) string {
	return strings.TrimRight(base, "/") + "/a/" + token
}

// Links mints both action tokens for one occurrence and returns their absolute
// URLs. It exists so a notification sender can attach native buttons without
// duplicating the mint-and-compose steps.
func Links(secret []byte, base string, eventID int, occasion time.Time, userID int, ttl time.Duration) (snoozeURL, ackURL string, err error) {
	snoozeToken, err := Mint(secret, eventID, occasion, userID, ActionSnooze, ttl)
	if err != nil {
		return "", "", err
	}
	ackToken, err := Mint(secret, eventID, occasion, userID, ActionAck, ttl)
	if err != nil {
		return "", "", err
	}
	return URL(base, snoozeToken), URL(base, ackToken), nil
}

// mac returns the raw HMAC-SHA256 of body under secret.
func mac(secret []byte, body string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(body))
	return m.Sum(nil)
}
