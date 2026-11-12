package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLocaleKeyParity asserts that every shipped locale catalog carries the
// same key set as en. A missing translation silently falls back to English, so
// parity must be checked in CI rather than noticed in the UI.
func TestLocaleKeyParity(t *testing.T) {
	dir := filepath.Join("..", "i18n", "locales")
	readCatalog := func(t *testing.T, name string) map[string]string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if len(m) == 0 {
			t.Fatalf("%s contains no keys", name)
		}
		return m
	}

	en := readCatalog(t, "en.json")
	de := readCatalog(t, "de.json")

	if len(en) != len(de) {
		t.Errorf("key count mismatch: en=%d de=%d", len(en), len(de))
	}
	for k, v := range en {
		dv, ok := de[k]
		if !ok {
			t.Errorf("de.json is missing key %q (en: %q)", k, v)
			continue
		}
		if dv == "" {
			t.Errorf("de.json has an empty value for %q", k)
		}
	}
	for k := range de {
		if _, ok := en[k]; !ok {
			t.Errorf("de.json has extra key %q not present in en.json", k)
		}
	}

	// The keys this feature added must exist in both catalogs.
	for _, k := range []string{
		"people.no_gift_idea",
		"stats.gift_spend",
		"stats.gift_total",
		"stats.gift_status",
		"stats.gift_count",
		"stats.gift_amount",
		"stats.gift_idea",
		"stats.gift_purchased",
		"stats.gift_given",
		"stats.gift_archived",
		"stats.gift_top_spenders",
		"stats.gift_none",
		"stats.gift_priced",
		"stats.gift_time_hint",
	} {
		if _, ok := en[k]; !ok {
			t.Errorf("en.json is missing new key %q", k)
		}
		if _, ok := de[k]; !ok {
			t.Errorf("de.json is missing new key %q", k)
		}
	}
}
