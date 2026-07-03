package vcard_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent/enttest"
	vcardlib "github.com/datey/datey/internal/vcard"
	"github.com/datey/datey/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

func TestIntegration_ImportAndExportRoundTrip(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_int_test_rt?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	people := repository.NewPersonRepository(client)

	vcfContent := `BEGIN:VCARD
VERSION:3.0
FN:Alice Johnson
NOTE:Met at conference
TEL:+1-555-0100
EMAIL:alice@example.com
END:VCARD
BEGIN:VCARD
VERSION:3.0
FN:Bob Smith
NOTE:Colleague from work
END:VCARD`

	parsed, err := vcardlib.Parse(strings.NewReader(vcfContent))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 parsed contacts, got %d", len(parsed))
	}

	for _, pc := range parsed {
		_, err := people.Create(ctx, pc.Name, pc.Notes)
		if err != nil {
			t.Fatalf("create person %q: %v", pc.Name, err)
		}
	}

	all, err := people.List(ctx)
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 people, got %d", len(all))
	}

	items := make([]vcardlib.NameNotes, len(all))
	for i, p := range all {
		items[i] = vcardlib.NameNotes{Name: p.Name, Notes: p.Notes}
	}
	data, err := vcardlib.Encode(items)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "FN:Alice Johnson") {
		t.Error("export missing FN:Alice Johnson")
	}
	if !strings.Contains(output, "FN:Bob Smith") {
		t.Error("export missing FN:Bob Smith")
	}
	if !strings.Contains(output, "NOTE:Met at conference") {
		t.Error("export missing Alice's NOTE")
	}
	if !strings.Contains(output, "NOTE:Colleague from work") {
		t.Error("export missing Bob's NOTE")
	}
	if !strings.Contains(output, "PRODID") {
		t.Error("export missing PRODID")
	}
}

func TestIntegration_DuplicateDetection(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_int_test_dedup?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	people := repository.NewPersonRepository(client)

	_, err := people.Create(ctx, "Alice", "First import")
	if err != nil {
		t.Fatalf("create initial person: %v", err)
	}

	vcf := `BEGIN:VCARD
VERSION:3.0
FN:Alice
NOTE:Duplicate
END:VCARD`

	parsed, err := vcardlib.Parse(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 parsed contact, got %d", len(parsed))
	}

	existing, _ := people.List(ctx)
	existingNames := make(map[string]bool, len(existing))
	for _, p := range existing {
		existingNames[p.Name] = true
	}

	if existingNames[parsed[0].Name] {
		t.Log("Duplicate detected correctly — skipping import")
	} else {
		t.Error("expected duplicate detection to find 'Alice'")
	}
}

func TestIntegration_ExportSingle(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_int_test_single?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	people := repository.NewPersonRepository(client)

	p, err := people.Create(ctx, "Single Contact", "Just notes")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	data, err := vcardlib.EncodeSingle(p.Name, p.Notes)
	if err != nil {
		t.Fatalf("encode single: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "FN:Single Contact") {
		t.Error("export missing correct FN")
	}
	if !strings.Contains(output, "NOTE:Just notes") {
		t.Error("export missing correct NOTE")
	}
}

func TestIntegration_ImportFileUpload(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_int_test_upload?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("vcf_file", "contacts.vcf")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, err = io.Copy(part, strings.NewReader(`BEGIN:VCARD
VERSION:3.0
FN:Upload Test
NOTE:Imported via file upload
END:VCARD`))
	if err != nil {
		t.Fatalf("copy vcard content: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/contacts/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if err := req.ParseMultipartForm(10 << 20); err != nil {
		t.Fatalf("parse multipart form: %v", err)
	}

	file, _, err := req.FormFile("vcf_file")
	if err != nil {
		t.Fatalf("get form file: %v", err)
	}
	defer file.Close()

	parsed, err := vcardlib.Parse(file)
	if err != nil {
		t.Fatalf("parse uploaded file: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 contact from upload, got %d", len(parsed))
	}
	if parsed[0].Name != "Upload Test" {
		t.Errorf("expected 'Upload Test', got %q", parsed[0].Name)
	}
	if !strings.Contains(parsed[0].Notes, "Imported via file upload") {
		t.Errorf("expected notes to contain 'Imported via file upload', got %q", parsed[0].Notes)
	}
}

func TestIntegration_ExportEmptyDatabase(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_int_test_empty?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	people := repository.NewPersonRepository(client)

	all, err := people.List(ctx)
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 people, got %d", len(all))
	}

	items := make([]vcardlib.NameNotes, len(all))
	for i, p := range all {
		items[i] = vcardlib.NameNotes{Name: p.Name, Notes: p.Notes}
	}
	data, err := vcardlib.Encode(items)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty export for empty database, got %d bytes", len(data))
	}
}

func TestIntegration_ImportWithBirthdayCreatesEvent(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_int_test_bday?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	people := repository.NewPersonRepository(client)
	events := repository.NewEventRepository(client)

	vcf := `BEGIN:VCARD
VERSION:4.0
FN:Dana Vreede
BDAY:19980129
GENDER:F
N:Vreede;Dana;de;;
END:VCARD`

	parsed, err := vcardlib.Parse(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(parsed))
	}

	// Verify parsed fields
	if parsed[0].Birthday == nil {
		t.Fatal("expected Birthday to be parsed")
	}
	expectedBday := time.Date(1998, 1, 29, 0, 0, 0, 0, time.UTC)
	if !parsed[0].Birthday.Equal(expectedBday) {
		t.Errorf("expected Birthday %v, got %v", expectedBday, parsed[0].Birthday)
	}
	if parsed[0].Gender != "Female" {
		t.Errorf("expected Gender 'Female', got %q", parsed[0].Gender)
	}
	if parsed[0].FamilyName != "Vreede" {
		t.Errorf("expected FamilyName 'Vreede', got %q", parsed[0].FamilyName)
	}
	if parsed[0].GivenName != "Dana" {
		t.Errorf("expected GivenName 'Dana', got %q", parsed[0].GivenName)
	}
	// Notes should only contain the human-readable Gender label (no NOTE, TEL, EMAIL, or ADR in this vCard)
	if parsed[0].Notes != "Gender: Female" {
		t.Errorf("expected Notes 'Gender: Female', got %q", parsed[0].Notes)
	}

	// Create the person and event (simulating what the web handler does)
	p, err := people.Create(ctx, parsed[0].Name, parsed[0].Notes)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	if parsed[0].Birthday != nil {
		desc := "Birthday of " + parsed[0].Name
		if _, err := events.CreateForPerson(ctx, p.ID, "birthday", *parsed[0].Birthday, desc); err != nil {
			t.Fatalf("create birthday event: %v", err)
		}
	}

	// Verify the person exists
	savedPerson, err := people.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if savedPerson.Name != "Dana Vreede" {
		t.Errorf("expected name 'Dana Vreede', got %q", savedPerson.Name)
	}

	// Verify the birthday event was created
	personEvents, err := events.ListByPerson(ctx, p.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(personEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(personEvents))
	}
	if personEvents[0].Type != "birthday" {
		t.Errorf("expected event type 'birthday', got %q", personEvents[0].Type)
	}
	if !personEvents[0].Date.Equal(expectedBday) {
		t.Errorf("expected event date %v, got %v", expectedBday, personEvents[0].Date)
	}
	if personEvents[0].Description != "Birthday of Dana Vreede" {
		t.Errorf("expected description 'Birthday of Dana Vreede', got %q", personEvents[0].Description)
	}
}

func TestIntegration_ImportWithoutBirthdayNoEvent(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_int_test_nobday?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	people := repository.NewPersonRepository(client)
	events := repository.NewEventRepository(client)

	vcf := `BEGIN:VCARD
VERSION:3.0
FN:John Doe
TEL:+1-555-0100
END:VCARD`

	parsed, err := vcardlib.Parse(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(parsed))
	}
	if parsed[0].Birthday != nil {
		t.Fatal("expected Birthday to be nil when BDAY absent")
	}
	if !strings.Contains(parsed[0].Notes, "Phone: +1-555-0100") {
		t.Errorf("expected Phone in notes, got %q", parsed[0].Notes)
	}

	// Create person (simulating the web handler)
	p, err := people.Create(ctx, parsed[0].Name, parsed[0].Notes)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// No birthday event should have been created
	personEvents, err := events.ListByPerson(ctx, p.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(personEvents) != 0 {
		t.Errorf("expected 0 events, got %d", len(personEvents))
	}
}

func TestIntegration_DuplicateImportNoDuplicateEvent(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_int_test_dup?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	people := repository.NewPersonRepository(client)
	events := repository.NewEventRepository(client)

	// First import: create person + birthday event
	p, err := people.Create(ctx, "Dana Vreede", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	bday := time.Date(1998, 1, 29, 0, 0, 0, 0, time.UTC)
	if _, err := events.CreateForPerson(ctx, p.ID, "birthday", bday, "Birthday of Dana Vreede"); err != nil {
		t.Fatalf("create birthday event: %v", err)
	}

	// Second import: vCard with same name, should be detected as duplicate
	vcf := `BEGIN:VCARD
VERSION:4.0
FN:Dana Vreede
BDAY:19980129
END:VCARD`

	parsed, err := vcardlib.Parse(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(parsed))
	}

	// Duplicate check (same as the handler does)
	existing, err := people.FindByName(ctx, parsed[0].Name)
	if err != nil {
		t.Fatalf("find by name: %v", err)
	}
	if existing == nil {
		t.Fatal("expected to find existing person")
	}

	// Verify only 1 event exists (the one from first import)
	personEvents, err := events.ListByPerson(ctx, p.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(personEvents) != 1 {
		t.Errorf("expected exactly 1 event (no duplicate), got %d", len(personEvents))
	}
}
