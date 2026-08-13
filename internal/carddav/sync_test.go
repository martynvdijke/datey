package carddav

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/settings"
	_ "github.com/mattn/go-sqlite3"
)

// fixtureServer is an in-memory CardDAV server used by the sync engine tests.
// It answers well-known discovery, sync-collection REPORTs and object
// GET/PUT/DELETE against a small contact store.
type fixtureServer struct {
	mu        sync.Mutex
	t         *testing.T
	bookPath  string
	baseURL   string // set once the httptest server is started
	contacts  map[string]string // href path -> vCard body
	etags     map[string]string // href path -> etag
	token     string            // next sync-token to report
	report    []syncReportResponse // responses to return on the next REPORT
	puts      []string          // recorded PUT hrefs
	putBodies []string          // recorded PUT bodies
	deleteCount int
}

func newFixtureServer(t *testing.T) *fixtureServer {
	f := &fixtureServer{
		t:        t,
		bookPath: "/remote.php/dav/addressbooks/user/contacts",
		contacts: map[string]string{},
		etags:    map[string]string{},
		token:    "urn:uuid:tok-initial",
	}
	return f
}

func (f *fixtureServer) serve() *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		book := f.bookPath + "/"
		switch {
		case r.Method == "GET" && r.URL.Path == "/.well-known/carddav":
			w.Header().Set("Location", book)
			w.WriteHeader(http.StatusFound)
		case r.Method == "REPORT" && r.URL.Path == f.bookPath:
			f.serveReport(w)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, book):
			body, ok := f.contacts[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/vcard")
			_, _ = w.Write([]byte(body))
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, book):
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			f.contacts[r.URL.Path] = string(body)
			f.etags[r.URL.Path] = fmt.Sprintf(`"%d"`, len(f.contacts))
			f.puts = append(f.puts, r.URL.Path)
			f.putBodies = append(f.putBodies, string(body))
			w.Header().Set("ETag", f.etags[r.URL.Path])
			w.WriteHeader(http.StatusCreated)
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, book):
			delete(f.contacts, r.URL.Path)
			delete(f.etags, r.URL.Path)
			f.deleteCount++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	f.baseURL = srv.URL
	return srv
}

func (f *fixtureServer) serveReport(w http.ResponseWriter) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0"?>`)
	sb.WriteString(`<d:multistatus xmlns:d="DAV:">`)
	for _, resp := range f.report {
		href := resp.Href
		if !strings.HasPrefix(href, "http") {
			href = f.baseURL + href
		}
		sb.WriteString("<d:response>")
		sb.WriteString("<d:href>" + href + "</d:href>")
		sb.WriteString("<d:status>HTTP/1.1 " + resp.Status + "</d:status>")
		sb.WriteString("</d:response>")
	}
	sb.WriteString("<d:sync-token>" + f.token + "</d:sync-token>")
	sb.WriteString("</d:multistatus>")
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(sb.String()))
}

// newTestSyncer wires a Syncer against an in-memory ent client and a fixture
// CardDAV server.
func newTestSyncer(t *testing.T, deletePolicy string) (*Syncer, *ent.Client, *fixtureServer) {
	t.Helper()
	dsn := fmt.Sprintf("file:test_carddav_sync_%s?mode=memory&cache=shared&_fk=1", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	client := enttest.Open(t, dialect.SQLite, dsn)
	f := newFixtureServer(t)
	srv := f.serve()
	t.Cleanup(srv.Close)

	store := settings.New(client)
	ctx := context.Background()
	if err := store.EnsureSeeded(ctx); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	cfg := &config.Config{
		CarddavEnabled:     true,
		CarddavURL:         srv.URL + f.bookPath + "/",
		CarddavUsername:    "testuser",
		CarddavPassword:    "secret",
		CarddavDeletePolicy: deletePolicy,
	}
	return NewSyncer(cfg, client, store), client, f
}

const aliceCard = `BEGIN:VCARD
VERSION:3.0
UID:u-alice
FN:Alice Example
N:Example;Alice;;;
BDAY:1990-01-02
REV:2026-01-01T00:00:00Z
END:VCARD
`

func TestSync_Pull_CreatesNewPersonAndBirthdayEvent(t *testing.T) {
	s, client, f := newTestSyncer(t, "keep")
	f.report = []syncReportResponse{
		{Href: "/remote.php/dav/addressbooks/user/contacts/alice.vcf", Status: "200 OK"},
	}
	f.contacts["/remote.php/dav/addressbooks/user/contacts/alice.vcf"] = aliceCard

	res, err := s.Sync(context.Background(), SyncFull, true)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.PulledCreated != 1 {
		t.Errorf("PulledCreated = %d, want 1", res.PulledCreated)
	}

	people, err := s.people.List(context.Background())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 1 || people[0].Name != "Alice Example" {
		t.Fatalf("expected one person 'Alice Example', got %+v", people)
	}
	if people[0].CarddavUID == nil || *people[0].CarddavUID != "u-alice" {
		t.Errorf("CarddavUID = %v, want u-alice", people[0].CarddavUID)
	}

	events, err := s.events.ListByPerson(context.Background(), people[0].ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "birthday" {
		t.Fatalf("expected one birthday event, got %+v", events)
	}
	want := time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)
	if !events[0].Date.Equal(want) {
		t.Errorf("birthday = %v, want %v", events[0].Date, want)
	}
	_ = client
}

func TestSync_Pull_YearlessBDAYUsesYearZero(t *testing.T) {
	s, _, f := newTestSyncer(t, "keep")
	card := `BEGIN:VCARD
VERSION:3.0
UID:u-bob
FN:Bob Yearless
BDAY:--03-15
END:VCARD
`
	f.report = []syncReportResponse{
		{Href: "/remote.php/dav/addressbooks/user/contacts/bob.vcf", Status: "200 OK"},
	}
	f.contacts["/remote.php/dav/addressbooks/user/contacts/bob.vcf"] = card

	if _, err := s.Sync(context.Background(), SyncFull, true); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	people, _ := s.people.List(context.Background())
	if len(people) != 1 {
		t.Fatalf("expected one person, got %d", len(people))
	}
	events, _ := s.events.ListByPerson(context.Background(), people[0].ID)
	if len(events) != 1 {
		t.Fatalf("expected one birthday event, got %+v", events)
	}
	// Year-less BDAY maps to year 0.
	if events[0].Date.Year() != 0 || events[0].Date.Month() != 3 || events[0].Date.Day() != 15 {
		t.Errorf("birthday = %v, want year 0 / Mar 15", events[0].Date)
	}
}

func TestSync_Pull_UpdatesExistingByUID(t *testing.T) {
	s, _, f := newTestSyncer(t, "keep")
	ctx := context.Background()

	p, err := s.people.Create(ctx, "Alice Example", "old notes", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.people.SetCarddavState(ctx, p.ID, "u-alice", "/remote.php/dav/addressbooks/user/contacts/alice.vcf", "", "", nil, false); err != nil {
		t.Fatalf("set carddav state: %v", err)
	}

	updated := strings.Replace(aliceCard, "FN:Alice Example", "FN:Alice New Name", 1)
	f.report = []syncReportResponse{
		{Href: "/remote.php/dav/addressbooks/user/contacts/alice.vcf", Status: "200 OK"},
	}
	f.contacts["/remote.php/dav/addressbooks/user/contacts/alice.vcf"] = updated

	res, err := s.Sync(ctx, SyncFull, true)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.PulledUpdated != 1 {
		t.Errorf("PulledUpdated = %d, want 1", res.PulledUpdated)
	}

	got, _ := s.people.Get(ctx, p.ID)
	if got.Name != "Alice New Name" {
		t.Errorf("name = %q, want Alice New Name", got.Name)
	}
	if got.CarddavPendingSync {
		t.Error("remote update should clear pending flag")
	}
}

func TestSync_Pull_RemoteDeletionKeepPolicyUnlinks(t *testing.T) {
	s, _, f := newTestSyncer(t, "keep")
	ctx := context.Background()

	p, err := s.people.Create(ctx, "Alice Example", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	href := f.baseURL + "/remote.php/dav/addressbooks/user/contacts/alice.vcf"
	if _, err := s.people.SetCarddavState(ctx, p.ID, "u-alice", href, "", "", nil, false); err != nil {
		t.Fatalf("set carddav state: %v", err)
	}

	f.report = []syncReportResponse{
		{Href: href, Status: "404 Not Found"},
	}

	res, err := s.Sync(ctx, SyncFull, true)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.PulledDeleted != 0 {
		t.Errorf("PulledDeleted = %d, want 0 under keep policy", res.PulledDeleted)
	}

	got, _ := s.people.Get(ctx, p.ID)
	if got.CarddavUID != nil {
		t.Errorf("expected person to be unlinked, CarddavUID = %v", got.CarddavUID)
	}
}

func TestSync_Pull_RemoteDeletionDeletePolicyRemovesPerson(t *testing.T) {
	s, _, f := newTestSyncer(t, "delete")
	ctx := context.Background()

	p, err := s.people.Create(ctx, "Alice Example", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	href := f.baseURL + "/remote.php/dav/addressbooks/user/contacts/alice.vcf"
	if _, err := s.people.SetCarddavState(ctx, p.ID, "u-alice", href, "", "", nil, false); err != nil {
		t.Fatalf("set carddav state: %v", err)
	}
	if _, err := s.events.CreateForPerson(ctx, p.ID, "birthday", time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC), "Birthday of Alice"); err != nil {
		t.Fatalf("create event: %v", err)
	}

	f.report = []syncReportResponse{
		{Href: href, Status: "404 Not Found"},
	}

	res, err := s.Sync(ctx, SyncFull, true)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.PulledDeleted != 1 {
		t.Errorf("PulledDeleted = %d, want 1 under delete policy", res.PulledDeleted)
	}

	people, _ := s.people.List(ctx)
	if len(people) != 0 {
		t.Errorf("expected person deleted, got %+v", people)
	}
}

func TestSync_Push_CreatesLocalPersonRemotely(t *testing.T) {
	s, _, f := newTestSyncer(t, "keep")
	ctx := context.Background()

	// Local person with a birthday event but no CardDAV linkage yet.
	p, err := s.people.Create(ctx, "Carol Local", "phone: 123", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.events.CreateForPerson(ctx, p.ID, "birthday", time.Date(1985, 7, 4, 0, 0, 0, 0, time.UTC), "Birthday of Carol"); err != nil {
		t.Fatalf("create event: %v", err)
	}
	if _, err := s.people.SetCarddavPendingSync(ctx, p.ID, true); err != nil {
		t.Fatalf("set pending: %v", err)
	}

	f.report = nil
	res, err := s.Sync(ctx, SyncFull, true)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.PushedCreated != 1 {
		t.Errorf("PushedCreated = %d, want 1", res.PushedCreated)
	}
	if len(f.puts) != 1 {
		t.Fatalf("expected one PUT, got %d", len(f.puts))
	}
	body := f.putBodies[0]
	if !strings.Contains(body, "FN:Carol Local") {
		t.Errorf("pushed vCard missing FN: %s", body)
	}
	if !strings.Contains(body, "BDAY:1985-07-04") {
		t.Errorf("pushed vCard missing BDAY: %s", body)
	}
	if !strings.Contains(body, "UID:") {
		t.Errorf("pushed vCard missing UID: %s", body)
	}

	got, _ := s.people.Get(ctx, p.ID)
	if got.CarddavUID == nil || *got.CarddavUID == "" {
		t.Errorf("expected CarddavUID after push, got %v", got.CarddavUID)
	}
	if got.CarddavPendingSync {
		t.Error("push should clear pending flag")
	}
}

func TestSync_Push_UpdatesPendingLocalChangeWithETag(t *testing.T) {
	s, _, f := newTestSyncer(t, "keep")
	ctx := context.Background()

	p, err := s.people.Create(ctx, "Carol Local", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	href := f.baseURL + f.bookPath + "/u-carol.vcf"
	if _, err := s.people.SetCarddavState(ctx, p.ID, "u-carol", href, `"etag-1"`, "", nil, false); err != nil {
		t.Fatalf("set carddav state: %v", err)
	}
	if _, err := s.people.SetCarddavPendingSync(ctx, p.ID, true); err != nil {
		t.Fatalf("set pending: %v", err)
	}
	// The server already has the resource with the matching etag.
	f.contacts[href] = "BEGIN:VCARD\nVERSION:3.0\nUID:u-carol\nFN:Carol Local\nEND:VCARD\n"

	res, err := s.Sync(ctx, SyncPush, true)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.PushedUpdated != 1 {
		t.Errorf("PushedUpdated = %d, want 1", res.PushedUpdated)
	}
	if f.deleteCount != 0 {
		t.Errorf("unexpected DELETE count %d", f.deleteCount)
	}
	got, _ := s.people.Get(ctx, p.ID)
	if got.CarddavPendingSync {
		t.Error("push should clear pending flag")
	}
}

func TestSync_Conflict_LocalPendingWins(t *testing.T) {
	s, _, f := newTestSyncer(t, "keep")
	ctx := context.Background()

	p, err := s.people.Create(ctx, "Alice Example", "local edit", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Simulate a recent local edit: UpdatedAt in the future relative to REV.
	// SetCarddavState resets UpdatedAt, so the timestamp must be set after it.
	if _, err := s.people.SetCarddavState(ctx, p.ID, "u-alice", "/remote.php/dav/addressbooks/user/contacts/alice.vcf", "", "", nil, true); err != nil {
		t.Fatalf("set carddav state: %v", err)
	}
	if _, err := s.client.Person.UpdateOneID(p.ID).SetUpdatedAt(time.Now().Add(2 * time.Hour)).Save(ctx); err != nil {
		t.Fatalf("set updated at: %v", err)
	}

	// Remote REV (2026-01-01) is older than the local edit.
	f.report = []syncReportResponse{
		{Href: "/remote.php/dav/addressbooks/user/contacts/alice.vcf", Status: "200 OK"},
	}
	f.contacts["/remote.php/dav/addressbooks/user/contacts/alice.vcf"] = aliceCard

	res, err := s.Sync(ctx, SyncFull, true)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.PulledUpdated != 0 {
		t.Errorf("PulledUpdated = %d, want 0 (local wins)", res.PulledUpdated)
	}
	got, _ := s.people.Get(ctx, p.ID)
	if got.Name != "Alice Example" {
		t.Errorf("name = %q, want local name preserved", got.Name)
	}
}

func TestSync_Conflict_RemoteNewerWins(t *testing.T) {
	s, _, f := newTestSyncer(t, "keep")
	ctx := context.Background()

	p, err := s.people.Create(ctx, "Alice Example", "local edit", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.people.SetCarddavState(ctx, p.ID, "u-alice", "/remote.php/dav/addressbooks/user/contacts/alice.vcf", "", "", nil, true); err != nil {
		t.Fatalf("set carddav state: %v", err)
	}
	// Local edit is old: UpdatedAt before the remote REV. Must be set after
	// SetCarddavState, which resets UpdatedAt to now.
	if _, err := s.client.Person.UpdateOneID(p.ID).SetUpdatedAt(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)).Save(ctx); err != nil {
		t.Fatalf("set updated at: %v", err)
	}

	updated := strings.Replace(aliceCard, "FN:Alice Example", "FN:Alice Remote Wins", 1)
	f.report = []syncReportResponse{
		{Href: "/remote.php/dav/addressbooks/user/contacts/alice.vcf", Status: "200 OK"},
	}
	f.contacts["/remote.php/dav/addressbooks/user/contacts/alice.vcf"] = updated

	res, err := s.Sync(ctx, SyncFull, true)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.PulledUpdated != 1 {
		t.Errorf("PulledUpdated = %d, want 1 (remote wins)", res.PulledUpdated)
	}
	got, _ := s.people.Get(ctx, p.ID)
	if got.Name != "Alice Remote Wins" {
		t.Errorf("name = %q, want remote name", got.Name)
	}
}

func TestSync_NoAddressBookURLConfigured(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:test_carddav_sync_no_url?mode=memory&cache=shared&_fk=1")
	store := settings.New(client)
	ctx := context.Background()
	_ = store.EnsureSeeded(ctx)

	s := NewSyncer(&config.Config{}, client, store)
	if _, err := s.Sync(ctx, SyncFull, true); err == nil {
		t.Fatal("expected error when no CardDAV URL is configured")
	}
}
