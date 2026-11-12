package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/internal/sharedlink"
	"github.com/go-chi/chi/v5"
)

const shareTestSecret = "share-test-feed-key"

func newShareHandler(t *testing.T) *Handler {
	t.Helper()
	h := newTestWebHandler(t)
	h.cfg.ICalFeedKey = shareTestSecret
	return h
}

func shareToken(t *testing.T, kind string, id int, now time.Time, ttl time.Duration) string {
	t.Helper()
	token, err := sharedlink.NewSigner([]byte(shareTestSecret)).Sign(kind, id, ttl, now)
	if err != nil {
		t.Fatalf("sign share token: %v", err)
	}
	return token
}

func setupShareViewRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.NotFound(h.notFound)
	r.Get("/s/{token}", h.handleShareView)
	return r
}

func getSharePage(t *testing.T, h *Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/s/"+token, nil)
	rr := httptest.NewRecorder()
	setupShareViewRouter(h).ServeHTTP(rr, req)
	return rr
}

func seedEvent(t *testing.T, h *Handler, personID int, eventType string, date time.Time, description string) {
	t.Helper()
	if _, err := h.events.CreateForPerson(context.Background(), personID, eventType, date, description); err != nil {
		t.Fatalf("seed %s event: %v", eventType, err)
	}
}

func TestShareView_PersonRendersUpcomingAndLeaksNoPrivateData(t *testing.T) {
	h := newShareHandler(t)
	ctx := context.Background()

	person := seedPerson(t, h, "Alice Secretname")
	if _, err := h.people.Update(ctx, person.ID, person.Name, "PRIVATE-NOTE-TEXT", ""); err != nil {
		t.Fatalf("set notes: %v", err)
	}
	if _, err := h.giftIdeas.Create(ctx, person.ID, "PRIVATE-GIFT-TITLE", "PRIVATE-GIFT-NOTES", nil, ""); err != nil {
		t.Fatalf("create gift idea: %v", err)
	}
	seedEvent(t, h, person.ID, "birthday", time.Date(1990, time.June, 15, 0, 0, 0, 0, time.UTC), "")
	seedEvent(t, h, person.ID, "anniversary", time.Date(2015, time.March, 1, 0, 0, 0, 0, time.UTC), "PRIVATE-EVENT-DESC")

	token := shareToken(t, sharedlink.KindPerson, person.ID, time.Now(), sharedlink.DefaultTTL)
	rr := getSharePage(t, h, token)
	body := rr.Body.String()

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, body)
	}
	for _, want := range []string{
		"Alice Secretname",
		"birthday",
		"anniversary",
		`data-share-banner`,
		`name="robots"`,
		`content="noindex, nofollow"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	for _, leaked := range []string{
		"PRIVATE-NOTE-TEXT",
		"PRIVATE-GIFT-TITLE",
		"PRIVATE-GIFT-NOTES",
		"PRIVATE-EVENT-DESC",
	} {
		if strings.Contains(body, leaked) {
			t.Errorf("private data %q leaked into share page", leaked)
		}
	}
	if got := rr.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := rr.Header().Get("X-Robots-Tag"); !strings.Contains(got, "noindex") {
		t.Errorf("X-Robots-Tag = %q, want noindex", got)
	}
}

func TestShareView_GroupListsOnlyMembers(t *testing.T) {
	h := newShareHandler(t)
	ctx := context.Background()

	group := seedGroup(t, h, "Close Family")
	alice := seedPerson(t, h, "Alice Member")
	bob := seedPerson(t, h, "Bob Member")
	outsider := seedPerson(t, h, "Carol Outsider")
	for _, p := range []int{alice.ID, bob.ID} {
		if err := h.groups.AddPerson(ctx, group.ID, p); err != nil {
			t.Fatalf("add member: %v", err)
		}
	}
	seedEvent(t, h, alice.ID, "birthday", time.Date(1988, time.January, 2, 0, 0, 0, 0, time.UTC), "")
	seedEvent(t, h, bob.ID, "birthday", time.Date(1979, time.February, 3, 0, 0, 0, 0, time.UTC), "")
	seedEvent(t, h, outsider.ID, "birthday", time.Date(1970, time.March, 4, 0, 0, 0, 0, time.UTC), "")

	token := shareToken(t, sharedlink.KindGroup, group.ID, time.Now(), sharedlink.DefaultTTL)
	rr := getSharePage(t, h, token)
	body := rr.Body.String()

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, body)
	}
	for _, want := range []string{"Close Family", "Alice Member", "Bob Member"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	if strings.Contains(body, "Carol Outsider") {
		t.Error("non-member listed on group share page")
	}
	if got := strings.Count(body, "data-share-member="); got != 2 {
		t.Errorf("member sections = %d, want 2", got)
	}
}

func TestShareView_InvalidTokensAllLookIdentical(t *testing.T) {
	h := newShareHandler(t)
	person := seedPerson(t, h, "Alice Secretname")
	seedEvent(t, h, person.ID, "birthday", time.Date(1990, time.June, 15, 0, 0, 0, 0, time.UTC), "")

	now := time.Now()
	expired := shareToken(t, sharedlink.KindPerson, person.ID, now.Add(-60*24*time.Hour), sharedlink.DefaultTTL)
	valid := shareToken(t, sharedlink.KindPerson, person.ID, now, sharedlink.DefaultTTL)
	parts := strings.SplitN(valid, ".", 2)
	flipped := []byte(parts[1])
	if flipped[0] == 'A' {
		flipped[0] = 'B'
	} else {
		flipped[0] = 'A'
	}
	badSignature := parts[0] + "." + string(flipped)

	cases := map[string]string{
		"expired":       expired,
		"bad signature": badSignature,
		"garbage":       "not-a-token",
		"missing":       "",
	}
	var firstBody string
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			rr := getSharePage(t, h, token)
			body := rr.Body.String()
			if rr.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rr.Code)
			}
			if strings.Contains(body, "Alice Secretname") {
				t.Error("share page leaked the person name for an invalid token")
			}
			if strings.Contains(body, "birthday") {
				t.Error("share page leaked event data for an invalid token")
			}
			// A request without any token is served by the router's not-found
			// handler; token-bearing requests set the noindex header.
			if token != "" {
				if got := rr.Header().Get("X-Robots-Tag"); !strings.Contains(got, "noindex") {
					t.Errorf("X-Robots-Tag = %q, want noindex", got)
				}
			}
			if firstBody == "" {
				firstBody = body
			} else if body != firstBody {
				t.Error("invalid-token responses differ, which reveals which check failed")
			}
		})
	}
}

func setupShareCreateRouter(h *Handler, withCSRF bool) chi.Router {
	r := chi.NewRouter()
	if withCSRF {
		r.Use(h.CSRF)
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	}
	r.Post("/people/{id}/share", h.handleShareCreatePerson)
	r.Post("/groups/{id}/share", h.handleShareCreateGroup)
	return r
}

func TestShareCreate_RequiresAuth(t *testing.T) {
	h := newShareHandler(t)
	person := seedPerson(t, h, "Alice")

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(h.Auth)
		r.Post("/people/{id}/share", h.handleShareCreatePerson)
	})

	req := httptest.NewRequest(http.MethodPost, "/people/"+strconv.Itoa(person.ID)+"/share", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 redirect to login", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestShareCreate_PersonReturnsHTMXSnippetWithURLAndExpiry(t *testing.T) {
	h := newShareHandler(t)
	person := seedPerson(t, h, "Alice")

	req := httptest.NewRequest(http.MethodPost, "/people/"+strconv.Itoa(person.ID)+"/share", strings.NewReader("days=7"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req = req.WithContext(withUserContext(req.Context()))
	rr := httptest.NewRecorder()
	setupShareCreateRouter(h, false).ServeHTTP(rr, req)
	body := rr.Body.String()

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, body)
	}
	if !strings.Contains(body, "/s/") {
		t.Fatalf("snippet has no share URL: %s", body)
	}
	if !strings.Contains(body, "data-share-expiry") {
		t.Errorf("snippet has no expiry: %s", body)
	}
	if !strings.Contains(body, "data-share-copy") {
		t.Errorf("snippet has no copy button: %s", body)
	}

	idx := strings.Index(body, "/s/")
	token := body[idx+len("/s/"):]
	if cut := strings.IndexAny(token, `"<`); cut >= 0 {
		token = token[:cut]
	}
	payload, err := sharedlink.NewSigner([]byte(shareTestSecret)).Parse(token, time.Now())
	if err != nil {
		t.Fatalf("snippet token does not verify: %v", err)
	}
	if payload.Kind != sharedlink.KindPerson || payload.ID != person.ID {
		t.Errorf("payload = %+v, want person/%d", payload, person.ID)
	}
	if got := time.Until(payload.Expires); got < 6*24*time.Hour || got > 7*24*time.Hour {
		t.Errorf("expiry in %v, want about 7 days", got)
	}
}

func TestShareCreate_TTLClampedTo90Days(t *testing.T) {
	h := newShareHandler(t)
	person := seedPerson(t, h, "Alice")

	req := httptest.NewRequest(http.MethodPost, "/people/"+strconv.Itoa(person.ID)+"/share", strings.NewReader("days=999"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req = req.WithContext(withUserContext(req.Context()))
	rr := httptest.NewRecorder()
	setupShareCreateRouter(h, false).ServeHTTP(rr, req)

	body := rr.Body.String()
	idx := strings.Index(body, "/s/")
	if idx < 0 {
		t.Fatalf("no share URL in snippet: %s", body)
	}
	token := body[idx+len("/s/"):]
	if cut := strings.IndexAny(token, `"<`); cut >= 0 {
		token = token[:cut]
	}
	payload, err := sharedlink.NewSigner([]byte(shareTestSecret)).Parse(token, time.Now())
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if got := time.Until(payload.Expires); got < 89*24*time.Hour || got > 90*24*time.Hour {
		t.Errorf("expiry in %v, want clamped to 90 days", got)
	}
}

func TestShareCreate_GroupSnippet(t *testing.T) {
	h := newShareHandler(t)
	group := seedGroup(t, h, "Family")

	req := httptest.NewRequest(http.MethodPost, "/groups/"+strconv.Itoa(group.ID)+"/share", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req = req.WithContext(withUserContext(req.Context()))
	rr := httptest.NewRecorder()
	setupShareCreateRouter(h, false).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "/s/") {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}
}

func TestShareCreate_NonHTMXRedirectsWithFlash(t *testing.T) {
	h := newShareHandler(t)
	person := seedPerson(t, h, "Alice")

	req := httptest.NewRequest(http.MethodPost, "/people/"+strconv.Itoa(person.ID)+"/share", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	rr := httptest.NewRecorder()
	setupShareCreateRouter(h, false).ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}
	loc := rr.Header().Get("Location")
	wantPrefix := "/people/" + strconv.Itoa(person.ID) + "?success="
	if !strings.HasPrefix(loc, wantPrefix) {
		t.Fatalf("Location = %q, want prefix %q", loc, wantPrefix)
	}
	flash, err := url.QueryUnescape(strings.TrimPrefix(loc, wantPrefix))
	if err != nil {
		t.Fatalf("unescape flash: %v", err)
	}
	if !strings.Contains(flash, "/s/") {
		t.Errorf("flash does not carry the share URL: %q", flash)
	}
}

func TestShareCreate_CSRFRejected(t *testing.T) {
	h := newShareHandler(t)
	person := seedPerson(t, h, "Alice")

	req := httptest.NewRequest(http.MethodPost, "/people/"+strconv.Itoa(person.ID)+"/share", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	rr := httptest.NewRecorder()
	setupShareCreateRouter(h, true).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

func TestShareCreate_CSRFValidTokenAccepted(t *testing.T) {
	h := newShareHandler(t)
	person := seedPerson(t, h, "Alice")

	router := setupShareCreateRouter(h, true)

	// A GET to any route establishes the double-submit cookie.
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	var csrfCookie *http.Cookie
	for _, c := range getW.Result().Cookies() {
		if c.Name == "csrf_token" {
			csrfCookie = c
		}
	}
	if csrfCookie == nil {
		t.Fatal("no csrf cookie issued")
	}

	body := strings.NewReader("csrf_token=" + csrfCookie.Value + "&days=5")
	req := httptest.NewRequest(http.MethodPost, "/people/"+strconv.Itoa(person.ID)+"/share", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(csrfCookie)
	req = req.WithContext(withUserContext(req.Context()))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "/s/") {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}
}
