package i18n

import "testing"

func TestT_FallbackAndPassthrough(t *testing.T) {
	if got := T("en", "nav.dashboard"); got != "Dashboard" {
		t.Fatalf("expected Dashboard got %q", got)
	}
	if got := T("de", "nav.people"); got != "Personen" {
		t.Fatalf("expected Personen got %q", got)
	}
	// fallback to en when missing in de (use a key only in en)
	// add a temporary check: unknown key in de falls back to en or passthrough
	if got := T("de", "nav.dashboard"); got != "Dashboard" {
		t.Fatalf("fallback failed got %q", got)
	}
	// unknown key passthrough
	if got := T("en", "unknown.key.xyz"); got != "unknown.key.xyz" {
		t.Fatalf("expected passthrough got %q", got)
	}
	if got := T("de", "unknown.key.xyz"); got != "unknown.key.xyz" {
		t.Fatalf("expected passthrough got %q", got)
	}
}

func TestResolveLocale(t *testing.T) {
	tests := []struct {
		name   string
		user   string
		header string
		want   string
	}{
		{"user pref wins", "de", "en", "de"},
		{"header when no pref", "", "de", "de"},
		{"header en", "", "en", "en"},
		{"fallback en", "", "", "en"},
		{"unsupported header fallback", "", "fr", "en"},
		{"user unsupported fallback to header", "fr", "de", "de"},
		{"header with quality", "", "de;q=0.9, en;q=0.8", "de"},
		{"header region variant", "", "de-DE", "de"},
	}
	for _, tc := range tests {
		if got := ResolveLocale(tc.user, tc.header); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestNormalizeLocale(t *testing.T) {
	if got := NormalizeLocale("de-DE"); got != "de" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeLocale("EN"); got != "en" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeLocale("fr"); got != "fr" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeLocale("invalid"); got != "" {
		t.Fatalf("expected empty got %q", got)
	}
}
