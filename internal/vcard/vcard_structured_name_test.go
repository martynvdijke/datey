package vcard

import (
	"strings"
	"testing"
)

// structuredCard builds a vCard body with the given N and FN values.
func structuredCard(n, fn string) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\r\nVERSION:3.0\r\n")
	if n != "" {
		b.WriteString("N:" + n + "\r\n")
	}
	if fn != "" {
		b.WriteString("FN:" + fn + "\r\n")
	}
	b.WriteString("END:VCARD\r\n")
	return b.String()
}

func TestToContact_MiddleName(t *testing.T) {
	contacts, err := Parse(strings.NewReader(structuredCard("Doe;John;Q;;;", "John Q Doe")))
	if err != nil || len(contacts) != 1 {
		t.Fatalf("parse failed: %v (%d contacts)", err, len(contacts))
	}
	pc := contacts[0]
	if pc.GivenName != "John" || pc.MiddleName != "Q" || pc.FamilyName != "Doe" {
		t.Errorf("structured parts = (%q,%q,%q), want (John,Q,Doe)", pc.GivenName, pc.MiddleName, pc.FamilyName)
	}
	if pc.Name != "John Q Doe" {
		t.Errorf("display name = %q, want %q", pc.Name, "John Q Doe")
	}
}

func TestToContact_NWithoutFN(t *testing.T) {
	contacts, err := Parse(strings.NewReader(structuredCard("Public;John;Q;;;", "")))
	if err != nil || len(contacts) != 1 {
		t.Fatalf("parse failed: %v (%d contacts)", err, len(contacts))
	}
	pc := contacts[0]
	if pc.Name != "John Q Public" {
		t.Errorf("reconstructed display = %q, want %q", pc.Name, "John Q Public")
	}
}

func TestEncodeContact_EmitsStructuredN(t *testing.T) {
	data, err := EncodeContact(SyncContact{
		Name:       "Ada Lovelace",
		FirstName:  "Ada",
		MiddleName: "",
		LastName:   "Lovelace",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "N:Lovelace;Ada;;;") {
		t.Errorf("expected structured N in output:\n%s", body)
	}
	if !strings.Contains(body, "FN:Ada Lovelace") {
		t.Errorf("expected FN in output:\n%s", body)
	}
}

func TestEncodeContact_WithMiddleName(t *testing.T) {
	data, err := EncodeContact(SyncContact{
		Name:       "John Quincy Public",
		FirstName:  "John",
		MiddleName: "Quincy",
		LastName:   "Public",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(string(data), "N:Public;John;Quincy") {
		t.Errorf("expected N with middle name:\n%s", data)
	}
}

func TestEncodeContact_NoPartsOmitsN(t *testing.T) {
	data, err := EncodeContact(SyncContact{Name: "Solo"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(string(data), "\nN:") || strings.Contains(string(data), "\r\nN:") {
		t.Errorf("did not expect N when no structured parts:\n%s", data)
	}
}

// TestStructuredNameRoundTrip verifies import → export → re-parse preserves
// the first/middle/last split.
func TestStructuredNameRoundTrip(t *testing.T) {
	contacts, err := Parse(strings.NewReader(structuredCard("Doe;Jane;Alice;;;", "")))
	if err != nil || len(contacts) != 1 {
		t.Fatalf("parse failed: %v", err)
	}
	pc := contacts[0]

	data, err := EncodeContact(SyncContact{
		Name:       DisplayNameForTest(pc),
		FirstName:  pc.GivenName,
		MiddleName: pc.MiddleName,
		LastName:   pc.FamilyName,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	re, err := Parse(strings.NewReader(string(data)))
	if err != nil || len(re) != 1 {
		t.Fatalf("re-parse failed: %v", err)
	}
	got := re[0]
	if got.GivenName != "Jane" || got.MiddleName != "Alice" || got.FamilyName != "Doe" {
		t.Errorf("round-trip parts = (%q,%q,%q), want (Jane,Alice,Doe)", got.GivenName, got.MiddleName, got.FamilyName)
	}
	if got.Name != "Jane Alice Doe" {
		t.Errorf("round-trip display = %q, want %q", got.Name, "Jane Alice Doe")
	}
}

// DisplayNameForTest joins the parsed contact's structured parts the same way
// the app derives display names (given additional family).
func DisplayNameForTest(pc ParsedContact) string {
	parts := []string{}
	for _, p := range []string{pc.GivenName, pc.MiddleName, pc.FamilyName} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}
