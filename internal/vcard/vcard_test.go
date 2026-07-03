package vcard

import (
	"strings"
	"testing"
	"time"

	govcard "github.com/emersion/go-vcard"
)

func TestParse_SingleContact(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:3.0
FN:John Doe
NOTE:Met at conference
TEL:+1-555-0100
EMAIL:john@example.com
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].Name != "John Doe" {
		t.Errorf("expected Name 'John Doe', got %q", contacts[0].Name)
	}
	if !strings.Contains(contacts[0].Notes, "Met at conference") {
		t.Errorf("expected notes to contain 'Met at conference', got %q", contacts[0].Notes)
	}
	if !strings.Contains(contacts[0].Notes, "Phone: +1-555-0100") {
		t.Errorf("expected notes to contain 'Phone: +1-555-0100', got %q", contacts[0].Notes)
	}
	if !strings.Contains(contacts[0].Notes, "Email: john@example.com") {
		t.Errorf("expected notes to contain 'Email: john@example.com', got %q", contacts[0].Notes)
	}
}

func TestParse_MultiContact(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:3.0
FN:Alice
END:VCARD
BEGIN:VCARD
VERSION:3.0
FN:Bob
NOTE:Colleague
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(contacts))
	}
	if contacts[0].Name != "Alice" {
		t.Errorf("expected first contact 'Alice', got %q", contacts[0].Name)
	}
	if contacts[1].Name != "Bob" {
		t.Errorf("expected second contact 'Bob', got %q", contacts[1].Name)
	}
	if !strings.Contains(contacts[1].Notes, "Colleague") {
		t.Errorf("expected Bob's notes to contain 'Colleague', got %q", contacts[1].Notes)
	}
}

func TestParse_EmptyFile(t *testing.T) {
	contacts, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contacts != nil {
		t.Fatalf("expected nil for empty file, got %v", contacts)
	}
}

func TestParse_NoValidEntries(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:3.0
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contacts != nil {
		t.Fatalf("expected nil for contact with no FN, got %v", contacts)
	}
}

func TestParse_MalformedEntry(t *testing.T) {
	input := `NOT A VCARD
BEGIN:VCARD
VERSION:3.0
FN:Valid
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 valid contact, got %d", len(contacts))
	}
	if contacts[0].Name != "Valid" {
		t.Errorf("expected name 'Valid', got %q", contacts[0].Name)
	}
}

func TestToContact_FNandNOTE(t *testing.T) {
	card := make(govcard.Card)
	card.SetValue(govcard.FieldVersion, "3.0")
	card.SetValue(govcard.FieldFormattedName, "Jane Smith")
	card.SetValue(govcard.FieldNote, "Friend from work")

	pc := ToContact(card)
	if pc.Name != "Jane Smith" {
		t.Errorf("expected 'Jane Smith', got %q", pc.Name)
	}
	if !strings.Contains(pc.Notes, "Friend from work") {
		t.Errorf("expected notes 'Friend from work', got %q", pc.Notes)
	}
}

func TestToContact_OnlyFN(t *testing.T) {
	card := make(govcard.Card)
	card.SetValue(govcard.FieldVersion, "3.0")
	card.SetValue(govcard.FieldFormattedName, "Solo")

	pc := ToContact(card)
	if pc.Name != "Solo" {
		t.Errorf("expected 'Solo', got %q", pc.Name)
	}
	if pc.Notes != "" {
		t.Errorf("expected empty notes, got %q", pc.Notes)
	}
}

func TestToContact_UnrecognizedProps(t *testing.T) {
	card := make(govcard.Card)
	card.SetValue(govcard.FieldVersion, "3.0")
	card.SetValue(govcard.FieldFormattedName, "Full")
	card.SetValue(govcard.FieldTelephone, "+1-555-0100")
	card.SetValue(govcard.FieldEmail, "full@example.com")
	card.AddValue(govcard.FieldEmail, "alt@example.com")

	pc := ToContact(card)
	if !strings.Contains(pc.Notes, "Phone: +1-555-0100") {
		t.Errorf("expected Phone in notes, got %q", pc.Notes)
	}
	if !strings.Contains(pc.Notes, "Email: full@example.com") {
		t.Errorf("expected Email 'full@example.com' in notes, got %q", pc.Notes)
	}
}

func TestParse_BDAY(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:4.0
FN:Dana Vreede
BDAY:19980129
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].Birthday == nil {
		t.Fatal("expected Birthday to be non-nil")
	}
	expected := time.Date(1998, 1, 29, 0, 0, 0, 0, time.UTC)
	if !contacts[0].Birthday.Equal(expected) {
		t.Errorf("expected Birthday %v, got %v", expected, contacts[0].Birthday)
	}
}

func TestParse_BDAY_Extended(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:3.0
FN:Jane Doe
BDAY:1998-01-29
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].Birthday == nil {
		t.Fatal("expected Birthday to be non-nil")
	}
	expected := time.Date(1998, 1, 29, 0, 0, 0, 0, time.UTC)
	if !contacts[0].Birthday.Equal(expected) {
		t.Errorf("expected Birthday %v, got %v", expected, contacts[0].Birthday)
	}
}

func TestParse_BDAY_Absent(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:3.0
FN:No Birthday
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].Birthday != nil {
		t.Errorf("expected Birthday to be nil, got %v", contacts[0].Birthday)
	}
}

func TestParse_GENDER(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:4.0
FN:Dana Vreede
GENDER:F
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].Gender != "F" {
		t.Errorf("expected Gender 'F', got %q", contacts[0].Gender)
	}
}

func TestParse_N_StructuredName(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:4.0
FN:Dana Vreede
N:Vreede;Dana;de;;
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].FamilyName != "Vreede" {
		t.Errorf("expected FamilyName 'Vreede', got %q", contacts[0].FamilyName)
	}
	if contacts[0].GivenName != "Dana" {
		t.Errorf("expected GivenName 'Dana', got %q", contacts[0].GivenName)
	}
}

func TestParse_StructuredFieldsExcludedFromNotes(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:4.0
FN:Dana Vreede
BDAY:19980129
GENDER:F
N:Vreede;Dana;de;;
TEL:+1-555-0100
EMAIL:dana@example.com
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	// Notes should contain phone and email but NOT raw BDAY, GENDER, or N lines.
	if !strings.Contains(contacts[0].Notes, "Phone: +1-555-0100") {
		t.Errorf("expected Phone in notes, got %q", contacts[0].Notes)
	}
	if !strings.Contains(contacts[0].Notes, "Email: dana@example.com") {
		t.Errorf("expected Email in notes, got %q", contacts[0].Notes)
	}
	if strings.Contains(contacts[0].Notes, "BDAY") {
		t.Error("Notes should not contain raw BDAY")
	}
	if strings.Contains(contacts[0].Notes, "GENDER") {
		t.Error("Notes should not contain raw GENDER")
	}
	if strings.Contains(contacts[0].Notes, "N:") {
		t.Error("Notes should not contain raw N")
	}
}

func TestParse_UnknownFieldsPreservedInNotes(t *testing.T) {
	// Real-world vCards often have UID, SOURCE, PRODID, REV which are not
	// structured fields — they must be preserved in Notes to avoid data loss.
	input := `BEGIN:VCARD
VERSION:4.0
FN:Test User
UID:abc123
SOURCE:https://example.com/contact
PRODID:-//Test//EN
REV:20250131T084701Z
END:VCARD`

	contacts, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	if !strings.Contains(contacts[0].Notes, "UID: abc123") {
		t.Errorf("expected Notes to contain 'UID: abc123', got %q", contacts[0].Notes)
	}
	if !strings.Contains(contacts[0].Notes, "SOURCE: https://example.com/contact") {
		t.Errorf("expected Notes to contain SOURCE, got %q", contacts[0].Notes)
	}
	if !strings.Contains(contacts[0].Notes, "PRODID: -//Test//EN") {
		t.Errorf("expected Notes to contain PRODID, got %q", contacts[0].Notes)
	}
	if !strings.Contains(contacts[0].Notes, "REV: 20250131T084701Z") {
		t.Errorf("expected Notes to contain REV, got %q", contacts[0].Notes)
	}
}

func TestEncode_SingleContact(t *testing.T) {
	items := []NameNotes{
		{Name: "Alice", Notes: "Test note"},
	}

	data, err := Encode(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "BEGIN:VCARD") {
		t.Error("output missing BEGIN:VCARD")
	}
	if !strings.Contains(output, "END:VCARD") {
		t.Error("output missing END:VCARD")
	}
	if !strings.Contains(output, "VERSION:3.0") {
		t.Error("output missing VERSION:3.0")
	}
	if !strings.Contains(output, "FN:Alice") {
		t.Error("output missing FN:Alice")
	}
	if !strings.Contains(output, "NOTE:Test note") {
		t.Error("output missing NOTE:Test note")
	}
	if !strings.Contains(output, "PRODID:-//Datey//EN") {
		t.Error("output missing PRODID")
	}
}

func TestEncode_MultiContact(t *testing.T) {
	items := []NameNotes{
		{Name: "Alice", Notes: ""},
		{Name: "Bob", Notes: "Colleague"},
	}

	data, err := Encode(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "FN:Alice") {
		t.Error("output missing FN:Alice")
	}
	if !strings.Contains(output, "FN:Bob") {
		t.Error("output missing FN:Bob")
	}
}

func TestEncode_EmptyList(t *testing.T) {
	data, err := Encode([]NameNotes{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty output, got %d bytes", len(data))
	}
}

func TestEncodeSingle(t *testing.T) {
	data, err := EncodeSingle("Single", "Just me")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := string(data)
	if !strings.Contains(output, "FN:Single") {
		t.Error("output missing FN:Single")
	}
	if !strings.Contains(output, "NOTE:Just me") {
		t.Error("output missing NOTE:Just me")
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"John Doe", "John_Doe"},
		{"Alice/Bob", "Alice-Bob"},
		{"SimpleName", "SimpleName"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRoundTrip_ParseAndEncode(t *testing.T) {
	input := `BEGIN:VCARD
VERSION:3.0
FN:Round Trip
NOTE:Testing
END:VCARD`

	parsed, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(parsed))
	}

	items := []NameNotes{
		{Name: parsed[0].Name, Notes: parsed[0].Notes},
	}
	data, err := Encode(items)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "FN:Round Trip") {
		t.Errorf("round trip failed: FN not preserved")
	}
	if !strings.Contains(output, "NOTE:Testing") {
		t.Errorf("round trip failed: NOTE not preserved")
	}
}
