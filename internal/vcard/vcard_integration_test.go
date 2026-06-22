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

	part, err := writer.CreateFormFile("file", "contacts.vcf")
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

	file, _, err := req.FormFile("file")
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
