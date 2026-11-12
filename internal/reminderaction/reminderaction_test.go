package reminderaction

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

var testOccasion = time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)

func TestMintParseRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	tests := []struct {
		name   string
		action string
		userID int
	}{
		{"ack global", ActionAck, 0},
		{"ack user", ActionAck, 7},
		{"snooze global", ActionSnooze, 0},
		{"snooze user", ActionSnooze, 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := Mint(secret, 12, testOccasion, tt.userID, tt.action, DefaultTTL)
			if err != nil {
				t.Fatalf("Mint() error = %v", err)
			}
			got, err := Parse(secret, token)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got.EventID() != 12 {
				t.Errorf("EventID() = %d, want 12", got.EventID())
			}
			if got.UserID() != tt.userID {
				t.Errorf("UserID() = %d, want %d", got.UserID(), tt.userID)
			}
			if got.Kind() != tt.action {
				t.Errorf("Kind() = %q, want %q", got.Kind(), tt.action)
			}
			if !got.Occasion().Equal(testOccasion) {
				t.Errorf("Occasion() = %v, want %v", got.Occasion(), testOccasion)
			}
			if got.IsAck() != (tt.action == ActionAck) || got.IsSnooze() != (tt.action == ActionSnooze) {
				t.Errorf("IsAck/IsSnooze mismatch for %q", tt.action)
			}
			if got.Expires().IsZero() {
				t.Error("Expires() is zero")
			}
		})
	}
}

func TestMintTruncatesOccasionToDay(t *testing.T) {
	secret := []byte("s")
	occasion := time.Date(2026, 3, 14, 17, 45, 3, 9, time.FixedZone("CET", 3600))
	token, err := Mint(secret, 1, occasion, 0, ActionAck, DefaultTTL)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	got, err := Parse(secret, token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	// 17:45 CET on the 14th is 16:45 UTC on the 14th: same day, midnight UTC.
	if want := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC); !got.Occasion().Equal(want) {
		t.Errorf("Occasion() = %v, want %v", got.Occasion(), want)
	}
}

func TestParseRejectsTamperedToken(t *testing.T) {
	secret := []byte("test-secret")
	token, err := Mint(secret, 5, testOccasion, 1, ActionSnooze, DefaultTTL)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	body, sig, _ := strings.Cut(token, ".")

	// Flip the last character of the signature.
	last := sig[len(sig)-1]
	replacement := byte('A')
	if last == 'A' {
		replacement = 'B'
	}
	tamperedSig := sig[:len(sig)-1] + string(replacement)

	// Re-encode the payload with a different action, keeping the old signature.
	other, err := Mint(secret, 5, testOccasion, 1, ActionAck, DefaultTTL)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	otherBody, _, _ := strings.Cut(other, ".")

	cases := map[string]string{
		"tampered signature": body + "." + tamperedSig,
		"swapped payload":    otherBody + "." + sig,
		"missing signature":  body + ".",
		"missing payload":    "." + sig,
		"no separator":       body + sig,
	}
	for name, tok := range cases {
		if _, err := Parse(secret, tok); err == nil {
			t.Errorf("%s: Parse() accepted a token it must reject", name)
		}
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	token, err := Mint([]byte("secret-a"), 5, testOccasion, 0, ActionAck, DefaultTTL)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	if _, err := Parse([]byte("secret-b"), token); err != ErrBadSignature {
		t.Errorf("Parse() error = %v, want %v", err, ErrBadSignature)
	}
}

func TestParseExpiry(t *testing.T) {
	secret := []byte("s")
	token, err := Mint(secret, 3, testOccasion, 0, ActionAck, time.Minute)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}

	if _, err := ParseAt(secret, token, time.Now().Add(ClockSkew/2)); err != nil {
		t.Errorf("ParseAt() inside clock skew rejected token: %v", err)
	}
	if _, err := ParseAt(secret, token, time.Now().Add(2*ClockSkew+2*time.Minute)); err != ErrExpired {
		t.Errorf("ParseAt() error = %v, want %v", err, ErrExpired)
	}
}

func TestParseRejectsOversizedAndMalformed(t *testing.T) {
	secret := []byte("s")
	oversized := strings.Repeat("a", maxTokenLen+1)
	cases := map[string]string{
		"empty":     "",
		"oversized": oversized,
		"not b64":   "!!!.!!!",
	}
	for name, tok := range cases {
		if _, err := Parse(secret, tok); err != ErrMalformed {
			t.Errorf("%s: Parse() error = %v, want %v", name, err, ErrMalformed)
		}
	}

	// A syntactically valid signature over an oversized payload is still
	// rejected before unmarshalling.
	big := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", maxPayloadLen+1)))
	sig := base64.RawURLEncoding.EncodeToString(mac(secret, big))
	if _, err := Parse(secret, big+"."+sig); err != ErrMalformed {
		t.Errorf("oversized payload: Parse() error = %v, want %v", err, ErrMalformed)
	}
}

func TestMintValidation(t *testing.T) {
	cases := []struct {
		name   string
		secret []byte
		id     int
		user   int
		action string
		// wantErr is the sentinel expected; nil means the mint itself must
		// succeed (only the TTL bound differs).
		wantErr error
	}{
		{"no secret", nil, 1, 0, ActionAck, ErrNoSecret},
		{"empty id", []byte("s"), 0, 0, ActionAck, ErrMalformed},
		{"negative user", []byte("s"), 1, -1, ActionAck, ErrMalformed},
		{"unknown action", []byte("s"), 1, 0, "dismiss", ErrUnknown},
		{"empty action", []byte("s"), 1, 0, "", ErrUnknown},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Mint(tt.secret, tt.id, testOccasion, tt.user, tt.action, DefaultTTL); err != tt.wantErr {
				t.Errorf("Mint() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestClampTTL(t *testing.T) {
	cases := []struct {
		in, want time.Duration
	}{
		{0, DefaultTTL},
		{-time.Hour, DefaultTTL},
		{time.Minute, time.Minute},
		{MaxTTL + time.Hour, MaxTTL},
	}
	for _, c := range cases {
		if got := ClampTTL(c.in); got != c.want {
			t.Errorf("ClampTTL(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestLinks(t *testing.T) {
	snooze, ack, err := Links([]byte("s"), "https://datey.example/", 9, testOccasion, 2, DefaultTTL)
	if err != nil {
		t.Fatalf("Links() error = %v", err)
	}
	for name, url := range map[string]string{"snooze": snooze, "ack": ack} {
		if !strings.HasPrefix(url, "https://datey.example/a/") {
			t.Errorf("%s URL = %q, want prefix https://datey.example/a/", name, url)
		}
	}
	if snooze == ack {
		t.Error("snooze and ack URLs are identical")
	}
	// Each URL must parse back to the action it was minted for.
	snoozeToken := strings.TrimPrefix(snooze, "https://datey.example/a/")
	got, err := Parse([]byte("s"), snoozeToken)
	if err != nil {
		t.Fatalf("Parse(snooze) error = %v", err)
	}
	if !got.IsSnooze() {
		t.Errorf("snooze token Kind() = %q", got.Kind())
	}
}
