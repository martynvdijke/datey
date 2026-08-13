package carddav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testTransport is a plain HTTP transport that performs real requests against
// an httptest.Server. BasicAuthTransport is covered indirectly by the web
// handler tests; here the raw client behaviour is exercised end to end.
func newTestClient(t *testing.T, url string) *Client {
	t.Helper()
	return New(url, &BasicAuthTransport{Username: "user", Password: "pass"})
}

// carddavFixture wires a bare HTTP handler into a server that emulates a
// CardDAV server: well-known discovery, sync-collection REPORT, GET/PUT/DELETE.
func carddavFixture(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

const fixtureVCard = `BEGIN:VCARD
VERSION:3.0
UID:u-1234
FN:Alice Example
N:Example;Alice;;;
BDAY:1990-01-02
REV:2026-01-01T00:00:00Z
END:VCARD
`

func TestDiscover_WellKnownRedirect(t *testing.T) {
	srv := carddavFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/carddav":
			w.Header().Set("Location", "/remote.php/dav/addressbooks/user/contacts/")
			w.WriteHeader(http.StatusFound)
		case "/.well-known/caldav":
			t.Error("should not fall through to caldav when carddav succeeds")
			http.NotFound(w, r)
		case "/remote.php/dav/addressbooks/user/contacts/":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	})

	c := newTestClient(t, srv.URL)
	got, err := c.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := srv.URL + "/remote.php/dav/addressbooks/user/contacts/"
	if got != want {
		t.Errorf("Discover: got %q want %q", got, want)
	}
}

func TestDiscover_WellKnownFailsFallsBackToPROPFIND(t *testing.T) {
	srv := carddavFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PROPFIND" && r.URL.Path == "/":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:response>
    <d:href>/</d:href>
    <d:propstat>
      <d:prop>
        <card:addressbook-home-set>/dav/</card:addressbook-home-set>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`))
		case r.Method == "PROPFIND" && r.URL.Path == "/dav/":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:response>
    <d:href>/dav/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/></d:resourcetype>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/contacts/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/><card:addressbook/></d:resourcetype>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`))
		case r.Method == "PROPFIND" && r.URL.Path == "/dav/contacts/":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:response>
    <d:href>/dav/contacts/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/><card:addressbook/></d:resourcetype>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`))
		default:
			http.NotFound(w, r)
		}
	})

	c := newTestClient(t, srv.URL)
	got, err := c.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := srv.URL + "/dav/contacts/"
	if got != want {
		t.Errorf("Discover: got %q want %q", got, want)
	}
}

func TestSyncCollection_ReturnsResponsesAndToken(t *testing.T) {
	srv := carddavFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "REPORT" {
			t.Errorf("expected REPORT, got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/contacts/a.vcf</d:href>
    <d:status>HTTP/1.1 200 OK</d:status>
  </d:response>
  <d:response>
    <d:href>/contacts/b.vcf</d:href>
    <d:status>HTTP/1.1 404 Not Found</d:status>
  </d:response>
  <d:sync-token>urn:uuid:tok-1</d:sync-token>
</d:multistatus>`))
	})

	c := newTestClient(t, srv.URL+"/contacts/")
	report, err := c.SyncCollection(context.Background(), "")
	if err != nil {
		t.Fatalf("SyncCollection: %v", err)
	}
	if len(report.Responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(report.Responses))
	}
	if report.Responses[0].Href != "/contacts/a.vcf" {
		t.Errorf("response[0].Href = %q", report.Responses[0].Href)
	}
	if !strings.Contains(report.Responses[1].Status, "404") {
		t.Errorf("response[1] should be a 404 deletion, got %q", report.Responses[1].Status)
	}
	if report.SyncToken != "urn:uuid:tok-1" {
		t.Errorf("SyncToken = %q", report.SyncToken)
	}
}

func TestGet_ParsesVCard(t *testing.T) {
	srv := carddavFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contacts/a.vcf" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/vcard")
		_, _ = w.Write([]byte(fixtureVCard))
	})

	c := newTestClient(t, srv.URL)
	data, err := c.Get(context.Background(), srv.URL+"/contacts/a.vcf")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(string(data), "Alice Example") {
		t.Errorf("expected vCard body, got %q", data)
	}
}

func TestPut_UsesConditionalHeaders(t *testing.T) {
	srv := carddavFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		// Create: If-None-Match: * required.
		if im := r.Header.Get("If-Match"); im != "" {
			t.Errorf("create should not set If-Match, got %q", im)
		}
		if inm := r.Header.Get("If-None-Match"); inm != "*" {
			t.Errorf("create should set If-None-Match: *, got %q", inm)
		}
		w.Header().Set("ETag", `"etag-new"`)
		w.WriteHeader(http.StatusCreated)
	})

	c := newTestClient(t, srv.URL)
	res, err := c.Put(context.Background(), srv.URL+"/contacts/c.vcf", []byte(fixtureVCard), "")
	if err != nil {
		t.Fatalf("Put (create): %v", err)
	}
	if res.ETag != `"etag-new"` {
		t.Errorf("Put ETag = %q", res.ETag)
	}
}

func TestPut_UpdateSendsIfMatch(t *testing.T) {
	srv := carddavFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if im := r.Header.Get("If-Match"); im != `"etag-old"` {
			t.Errorf("update should set If-Match: \"etag-old\", got %q", im)
		}
		w.Header().Set("ETag", `"etag-new"`)
		w.WriteHeader(http.StatusNoContent)
	})

	c := newTestClient(t, srv.URL)
	res, err := c.Put(context.Background(), srv.URL+"/contacts/c.vcf", []byte(fixtureVCard), `"etag-old"`)
	if err != nil {
		t.Fatalf("Put (update): %v", err)
	}
	if res.ETag != `"etag-new"` {
		t.Errorf("Put ETag = %q", res.ETag)
	}
}

func TestPut_PreconditionFailed(t *testing.T) {
	srv := carddavFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	})

	c := newTestClient(t, srv.URL)
	if _, err := c.Put(context.Background(), srv.URL+"/contacts/c.vcf", []byte(fixtureVCard), `"etag-old"`); err == nil {
		t.Fatal("expected error on 412")
	}
}

func TestDelete_Confirmation(t *testing.T) {
	srv := carddavFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	c := newTestClient(t, srv.URL)
	deleted, err := c.Delete(context.Background(), srv.URL+"/contacts/gone.vcf")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Error("expected deleted=true on 204")
	}
}

func TestDelete_AlreadyGone(t *testing.T) {
	srv := carddavFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	c := newTestClient(t, srv.URL)
	deleted, err := c.Delete(context.Background(), srv.URL+"/contacts/gone.vcf")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if deleted {
		t.Error("expected deleted=false on 404")
	}
}
