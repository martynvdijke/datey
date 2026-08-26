package vcard

import (
	"strings"
	"testing"
)

func TestEncode_Categories_ExportAndImport(t *testing.T) {
	items := []NameNotes{
		{Name: "Alice", Notes: "hello", Categories: []string{"Family", "Friends"}},
	}
	data, err := Encode(items)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "CATEGORIES:Family,Friends") {
		t.Errorf("expected CATEGORIES line, got %q", s)
	}
	contacts, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	if len(contacts[0].Categories) != 2 || contacts[0].Categories[0] != "Family" || contacts[0].Categories[1] != "Friends" {
		t.Errorf("Categories = %v, want [Family Friends]", contacts[0].Categories)
	}
}

func TestEncode_Categories_CommaEscaped(t *testing.T) {
	items := []NameNotes{
		{Name: "Bob", Notes: "", Categories: []string{"Family, Extended", "Work"}},
	}
	data, err := Encode(items)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "CATEGORIES:Family\\, Extended,Work") {
		t.Errorf("expected escaped comma, got %q", s)
	}
	contacts, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(contacts[0].Categories) != 2 || contacts[0].Categories[0] != "Family, Extended" {
		t.Errorf("Categories round-trip with comma failed: %v", contacts[0].Categories)
	}
}

func TestParse_Categories_Raw(t *testing.T) {
	input := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Carol\r\nCATEGORIES:Friends,Work\r\nEND:VCARD\r\n"
	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(contacts[0].Categories) != 2 {
		t.Fatalf("Categories = %v, want 2", contacts[0].Categories)
	}
}
