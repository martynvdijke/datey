package web

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/internal/rss"
	"github.com/go-chi/chi/v5"
)

func setupRSSRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/rss.xml", h.rssFeed)
	return r
}

func enableRSSFeed(h *Handler) {
	h.cfg.RSSEnabled = true
	h.cfg.RSSFeedKey = "testrsskey"
}

func TestRSSFeedDisabledReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupRSSRouter(h)

	req := httptest.NewRequest("GET", "/rss.xml?key=testrsskey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when feed disabled, got %d", w.Code)
	}
}

func TestRSSFeedMissingKeyReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableRSSFeed(h)
	router := setupRSSRouter(h)

	req := httptest.NewRequest("GET", "/rss.xml", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing key, got %d", w.Code)
	}
}

func TestRSSFeedWrongKeyReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableRSSFeed(h)
	router := setupRSSRouter(h)

	req := httptest.NewRequest("GET", "/rss.xml?key=wrongkey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong key, got %d", w.Code)
	}
}

func TestRSSFeedReturnsItems(t *testing.T) {
	h := newTestWebHandler(t)
	enableRSSFeed(h)
	h.cfg.ReminderDays = 7
	personID := newTestPerson(t, h, "Dana")
	newTestEvent(t, h, personID, "birthday", time.Now().AddDate(0, 0, 3))
	router := setupRSSRouter(h)

	req := httptest.NewRequest("GET", "/rss.xml?key=testrsskey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String()[:200])
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
		t.Errorf("expected application/rss+xml content type, got %q", ct)
	}

	var doc rss.RSS
	if err := xml.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("response is not valid RSS XML: %v\n%s", err, w.Body.String())
	}
	if doc.Version != "2.0" {
		t.Errorf("version = %q, want 2.0", doc.Version)
	}
	if len(doc.Channel.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(doc.Channel.Items))
	}
	item := doc.Channel.Items[0]
	if !strings.Contains(item.Title, "Birthday") || !strings.Contains(item.Title, "Dana") {
		t.Errorf("item title = %q, want 'Birthday — Dana in N days'", item.Title)
	}
	wantLink := "/people/" + strconv.Itoa(personID)
	if !strings.HasSuffix(item.Link, wantLink) {
		t.Errorf("item link = %q, want suffix %q", item.Link, wantLink)
	}
}

func TestRSSFeedEmptyFeed(t *testing.T) {
	h := newTestWebHandler(t)
	enableRSSFeed(h)
	router := setupRSSRouter(h)

	req := httptest.NewRequest("GET", "/rss.xml?key=testrsskey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var doc rss.RSS
	if err := xml.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("empty feed is not valid RSS XML: %v\n%s", err, w.Body.String())
	}
	if len(doc.Channel.Items) != 0 {
		t.Errorf("expected no items, got %d", len(doc.Channel.Items))
	}
}
