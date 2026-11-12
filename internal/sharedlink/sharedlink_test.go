package sharedlink

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("feed-key-used-as-share-secret")

func fixedNow() time.Time {
	return time.Date(2026, time.March, 14, 9, 30, 0, 0, time.UTC)
}

func mustSign(t *testing.T, kind string, id int, ttl time.Duration, now time.Time) string {
	t.Helper()
	token, err := NewSigner(testSecret).Sign(kind, id, ttl, now)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return token
}

func TestSignParseRoundTrip(t *testing.T) {
	now := fixedNow()
	token := mustSign(t, KindPerson, 42, DefaultTTL, now)

	p, err := NewSigner(testSecret).Parse(token, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Kind != KindPerson || p.ID != 42 {
		t.Fatalf("payload = %+v, want person/42", p)
	}
	if !p.Expires.Equal(now.Add(DefaultTTL)) {
		t.Errorf("expiry = %v, want %v", p.Expires, now.Add(DefaultTTL))
	}
	if p.Nonce == "" {
		t.Error("nonce is empty")
	}
}

func TestSignParseRoundTripGroup(t *testing.T) {
	now := fixedNow()
	token := mustSign(t, KindGroup, 7, 3*24*time.Hour, now)

	p, err := NewSigner(testSecret).Parse(token, now)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Kind != KindGroup || p.ID != 7 {
		t.Fatalf("payload = %+v, want group/7", p)
	}
}

func TestSign_DistinctNonces(t *testing.T) {
	now := fixedNow()
	a := mustSign(t, KindPerson, 1, DefaultTTL, now)
	b := mustSign(t, KindPerson, 1, DefaultTTL, now)
	if a == b {
		t.Fatal("two tokens minted for the same subject are identical; nonce missing")
	}
}

func TestParse_TamperedPayload(t *testing.T) {
	now := fixedNow()
	token := mustSign(t, KindPerson, 1, DefaultTTL, now)
	body, sig, _ := strings.Cut(token, ".")

	// Re-encode the payload with a different ID but keep the old signature.
	p := mustSign(t, KindPerson, 2, DefaultTTL, now)
	otherBody, _, _ := strings.Cut(p, ".")

	if _, err := NewSigner(testSecret).Parse(otherBody+"."+sig, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err = %v, want ErrBadSignature", err)
	}
	// Sanity: the untouched token still verifies.
	if _, err := NewSigner(testSecret).Parse(body+"."+sig, now); err != nil {
		t.Fatalf("original token rejected: %v", err)
	}
}

func TestParse_TamperedSignature(t *testing.T) {
	now := fixedNow()
	token := mustSign(t, KindPerson, 1, DefaultTTL, now)
	body, sig, _ := strings.Cut(token, ".")

	flipped := []byte(sig)
	if flipped[0] == 'A' {
		flipped[0] = 'B'
	} else {
		flipped[0] = 'A'
	}
	if _, err := NewSigner(testSecret).Parse(body+"."+string(flipped), now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err = %v, want ErrBadSignature", err)
	}
}

func TestParse_WrongSecret(t *testing.T) {
	now := fixedNow()
	token := mustSign(t, KindPerson, 1, DefaultTTL, now)
	if _, err := NewSigner([]byte("another-secret")).Parse(token, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err = %v, want ErrBadSignature", err)
	}
}

func TestParse_Expired(t *testing.T) {
	now := fixedNow()
	token := mustSign(t, KindPerson, 1, DefaultTTL, now)

	// Just past the expiry plus the documented skew.
	after := now.Add(DefaultTTL + ClockSkew + time.Second)
	if _, err := NewSigner(testSecret).Parse(token, after); !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
}

func TestParse_ExpirySkewTolerance(t *testing.T) {
	now := fixedNow()
	token := mustSign(t, KindPerson, 1, DefaultTTL, now)

	// A verifier whose clock is slightly ahead of the issuer still accepts a
	// token until ClockSkew after its expiry.
	justAfter := now.Add(DefaultTTL + ClockSkew/2)
	if _, err := NewSigner(testSecret).Parse(token, justAfter); err != nil {
		t.Fatalf("token inside skew window rejected: %v", err)
	}
}

func TestParse_FutureExpiryBeyondMax(t *testing.T) {
	now := fixedNow()
	token := mustSign(t, KindPerson, 1, DefaultTTL, now)

	// Simulate a verifier clock far behind the issuer: nothing may stay valid
	// longer than MaxTTL plus the skew allowance.
	wayBefore := now.Add(DefaultTTL - MaxTTL - 2*ClockSkew)
	if _, err := NewSigner(testSecret).Parse(token, wayBefore); !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed for an over-long lifetime", err)
	}
}

func TestParse_UnknownKind(t *testing.T) {
	now := fixedNow()
	if _, err := NewSigner(testSecret).Sign("planet", 1, DefaultTTL, now); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("Sign err = %v, want ErrUnknownKind", err)
	}

	// A correctly signed token carrying an unknown kind is rejected too.
	token, err := signRaw(wirePayload{Kind: "planet", ID: 1, Expires: now.Add(time.Hour).Unix(), Nonce: "x"})
	if err != nil {
		t.Fatalf("signRaw: %v", err)
	}
	if _, err := NewSigner(testSecret).Parse(token, now); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("Parse err = %v, want ErrUnknownKind", err)
	}
}

func TestParse_BadIdentifiers(t *testing.T) {
	now := fixedNow()
	if _, err := NewSigner(testSecret).Sign(KindPerson, 0, DefaultTTL, now); !errors.Is(err, ErrMalformed) {
		t.Fatalf("Sign id=0 err = %v, want ErrMalformed", err)
	}

	token, err := signRaw(wirePayload{Kind: KindPerson, ID: -3, Expires: now.Add(time.Hour).Unix(), Nonce: "x"})
	if err != nil {
		t.Fatalf("signRaw: %v", err)
	}
	if _, err := NewSigner(testSecret).Parse(token, now); !errors.Is(err, ErrMalformed) {
		t.Fatalf("Parse err = %v, want ErrMalformed", err)
	}

	noExpiry, err := signRaw(wirePayload{Kind: KindPerson, ID: 1, Expires: 0, Nonce: "x"})
	if err != nil {
		t.Fatalf("signRaw: %v", err)
	}
	if _, err := NewSigner(testSecret).Parse(noExpiry, now); !errors.Is(err, ErrMalformed) {
		t.Fatalf("Parse err = %v, want ErrMalformed", err)
	}
}

func TestParse_EmptyGarbageOversized(t *testing.T) {
	now := fixedNow()
	s := NewSigner(testSecret)

	cases := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"no separator", "abcdef"},
		{"empty body", ".c2ln"},
		{"empty signature", "cGF5bG9hZA=."},
		{"extra separator", "a.b.c"},
		{"garbage", "%%%not-base64%%%"},
		{"oversized", strings.Repeat("a", maxTokenLen+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Parse(tc.token, now); !errors.Is(err, ErrMalformed) {
				t.Fatalf("err = %v, want ErrMalformed", err)
			}
		})
	}
}

func TestParse_URLUnsafeBase64(t *testing.T) {
	now := fixedNow()

	// A body containing standard-alphabet characters ('+' or '/') cannot be
	// raw-url-decoded, even when the signature over the literal body is valid.
	body := "ab+cd/ef"
	token := body + "." + encodeSig(NewSigner(testSecret).mac(body))
	if _, err := NewSigner(testSecret).Parse(token, now); !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
}

func TestParse_NoSecret(t *testing.T) {
	now := fixedNow()
	s := NewSigner(nil)
	if _, err := s.Sign(KindPerson, 1, DefaultTTL, now); !errors.Is(err, ErrNoSecret) {
		t.Fatalf("Sign err = %v, want ErrNoSecret", err)
	}
	if _, err := s.Parse("x.y", now); !errors.Is(err, ErrNoSecret) {
		t.Fatalf("Parse err = %v, want ErrNoSecret", err)
	}
}

func TestClampTTL(t *testing.T) {
	cases := []struct {
		in, want time.Duration
	}{
		{0, DefaultTTL},
		{-time.Hour, DefaultTTL},
		{time.Hour, MinTTL},
		{48 * time.Hour, 48 * time.Hour},
		{365 * 24 * time.Hour, MaxTTL},
	}
	for _, tc := range cases {
		if got := ClampTTL(tc.in); got != tc.want {
			t.Errorf("ClampTTL(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSign_TTLClamped(t *testing.T) {
	now := fixedNow()

	token := mustSign(t, KindPerson, 1, 365*24*time.Hour, now)
	p, err := NewSigner(testSecret).Parse(token, now)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := p.Expires.Sub(now); got != MaxTTL {
		t.Errorf("long TTL clamped to %v, want %v", got, MaxTTL)
	}

	token = mustSign(t, KindPerson, 1, time.Minute, now)
	p, err = NewSigner(testSecret).Parse(token, now)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := p.Expires.Sub(now); got != MinTTL {
		t.Errorf("short TTL clamped to %v, want %v", got, MinTTL)
	}
}

// signRaw signs an arbitrary payload, bypassing Sign's validation, so Parse's
// own checks can be exercised on otherwise unreachable states.
func signRaw(w wirePayload) (string, error) {
	raw, err := json.Marshal(w)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	return body + "." + encodeSig(NewSigner(testSecret).mac(body)), nil
}

func encodeSig(sig []byte) string {
	return base64.RawURLEncoding.EncodeToString(sig)
}
