package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func setupDataQualityRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/data-quality", h.handleDataQuality)
	return r
}

func setupDataQualityAuthRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(h.Auth)
		r.Get("/data-quality", h.handleDataQuality)
	})
	return r
}

func getDataQualityPage(t *testing.T, h *Handler) (int, string) {
	t.Helper()
	router := setupDataQualityRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/data-quality", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr.Code, rr.Body.String()
}

func TestDataQuality_EmptyLibraryShowsSectionsAndEmptyStates(t *testing.T) {
	h := newTestWebHandler(t)

	code, body := getDataQualityPage(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", code, body)
	}
	for _, id := range []string{"dq-missing-birth-year", "dq-missing-photo", "dq-birthday-soon", "dq-duplicates"} {
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("section %q missing from page", id)
		}
	}
	for _, empty := range []string{
		"Everyone with a birthday has a birth year.",
		"Everyone has a photo.",
		"No upcoming birthday is missing a gift idea.",
		"No names look duplicated.",
	} {
		if !strings.Contains(body, empty) {
			t.Errorf("empty state %q missing from page", empty)
		}
	}
	if !strings.Contains(body, "0 of 0 people have no birth year") {
		t.Errorf("summary line missing or wrong: %s", body)
	}
	if !strings.Contains(body, "Datey - Data quality") {
		t.Errorf("page title missing: %s", body)
	}
}

func TestDataQuality_MissingBirthYear(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()

	yearless, err := h.people.Create(ctx, "No Year", "", "")
	if err != nil {
		t.Fatalf("create yearless person: %v", err)
	}
	if _, err := h.events.CreateForPerson(ctx, yearless.ID, "birthday", time.Date(0, 6, 8, 0, 0, 0, 0, time.UTC), ""); err != nil {
		t.Fatalf("create yearless birthday: %v", err)
	}
	withYear, err := h.people.Create(ctx, "Has Year", "", "")
	if err != nil {
		t.Fatalf("create person with year: %v", err)
	}
	if _, err := h.events.CreateForPerson(ctx, withYear.ID, "birthday", time.Date(1990, 6, 8, 0, 0, 0, 0, time.UTC), ""); err != nil {
		t.Fatalf("create birthday with year: %v", err)
	}
	// A person without any birthday event is not "missing a birth year".
	if _, err := h.people.Create(ctx, "No Birthday", "", ""); err != nil {
		t.Fatalf("create person without birthday: %v", err)
	}

	code, body := getDataQualityPage(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, "1 of 3 people have no birth year") {
		t.Errorf("missing-birth-year summary wrong: %s", body)
	}
	if !strings.Contains(body, `href="/people/`+itoa(yearless.ID)+`"`) {
		t.Errorf("yearless person link missing: %s", body)
	}
	if !strings.Contains(body, "no year") {
		t.Errorf("detail for yearless birthday missing: %s", body)
	}

	// The section must list exactly the yearless person, not the others.
	section := dqSection(body, "dq-missing-birth-year")
	if !strings.Contains(section, "No Year") {
		t.Errorf("yearless person missing from its section")
	}
	for _, excluded := range []string{"Has Year", "No Birthday"} {
		if strings.Contains(section, excluded) {
			t.Errorf("%q must not be listed as missing a birth year", excluded)
		}
	}
}

func TestDataQuality_MissingPhoto(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()

	noPhoto, err := h.people.Create(ctx, "No Photo", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	localPhoto, err := h.people.Create(ctx, "Local Photo", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := h.people.SetPhotoState(ctx, localPhoto.ID, "photos/1.jpg", "image/jpeg", "upload"); err != nil {
		t.Fatalf("set local photo: %v", err)
	}
	immichPhoto, err := h.people.Create(ctx, "Immich Photo", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	immichID := "immich-asset-1"
	if _, err := h.people.SetImmichPhoto(ctx, immichPhoto.ID, &immichID, false); err != nil {
		t.Fatalf("set immich photo: %v", err)
	}

	code, body := getDataQualityPage(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, "1 of 3 people have no photo") {
		t.Errorf("missing-photo summary wrong: %s", body)
	}
	section := dqSection(body, "dq-missing-photo")
	if !strings.Contains(section, `href="/people/`+itoa(noPhoto.ID)+`"`) {
		t.Errorf("person without photo missing from its section")
	}
	for _, excluded := range []string{"Local Photo", "Immich Photo"} {
		if strings.Contains(section, excluded) {
			t.Errorf("%q must not be listed as missing a photo", excluded)
		}
	}
}

func TestDataQuality_BirthdaySoonWithoutGiftIdea(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()

	// Ten days out, with a real birth year so this section is independent of
	// the missing-year rule.
	soon := time.Now().AddDate(0, 0, 10)
	soonDate := time.Date(1990, soon.Month(), soon.Day(), 0, 0, 0, 0, time.UTC)

	noGift, err := h.people.Create(ctx, "No Gift", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := h.events.CreateForPerson(ctx, noGift.ID, "birthday", soonDate, ""); err != nil {
		t.Fatalf("create birthday: %v", err)
	}
	withGift, err := h.people.Create(ctx, "With Gift", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := h.events.CreateForPerson(ctx, withGift.ID, "birthday", soonDate, ""); err != nil {
		t.Fatalf("create birthday: %v", err)
	}
	if _, err := h.giftIdeas.Create(ctx, withGift.ID, "Book", "", nil, ""); err != nil {
		t.Fatalf("create gift idea: %v", err)
	}
	// A far-away birthday is outside the window.
	far := time.Now().AddDate(0, 0, 100)
	farDate := time.Date(1990, far.Month(), far.Day(), 0, 0, 0, 0, time.UTC)
	farAway, err := h.people.Create(ctx, "Far Away", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := h.events.CreateForPerson(ctx, farAway.ID, "birthday", farDate, ""); err != nil {
		t.Fatalf("create birthday: %v", err)
	}

	code, body := getDataQualityPage(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, "1 of 3 people have a birthday within 45 days and no gift idea") {
		t.Errorf("birthday-soon summary wrong: %s", body)
	}
	section := dqSection(body, "dq-birthday-soon")
	if !strings.Contains(section, "No Gift") || !strings.Contains(section, "in 10 days") {
		t.Errorf("soon birthday missing from its section: %s", section)
	}
	for _, excluded := range []string{"With Gift", "Far Away"} {
		if strings.Contains(section, excluded) {
			t.Errorf("%q must not be listed as missing a gift idea", excluded)
		}
	}
}

func TestDataQuality_DuplicateNames(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()

	for _, name := range []string{"Jane Doe", "Doe, Jane", "jane  doe", "Alice Smith"} {
		if _, err := h.people.Create(ctx, name, "", ""); err != nil {
			t.Fatalf("create person %q: %v", name, err)
		}
	}

	code, body := getDataQualityPage(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, "1 possible duplicate group(s)") {
		t.Errorf("duplicate summary wrong: %s", body)
	}
	section := dqSection(body, "dq-duplicates")
	for _, name := range []string{"Jane Doe", "Doe, Jane", "jane  doe"} {
		if !strings.Contains(section, name) {
			t.Errorf("%q missing from duplicate cluster: %s", name, section)
		}
	}
	if strings.Contains(section, "Alice Smith") {
		t.Errorf("distinct name listed as a duplicate: %s", section)
	}
}

func TestDataQuality_NormalizedName(t *testing.T) {
	cases := []struct{ a, b string }{
		{"Jane Doe", "Doe, Jane"},
		{"Jane Doe", "  jane   doe "},
		{"Mary-Jane O'Neil", "O'Neil, Mary Jane"},
	}
	for _, c := range cases {
		if got, want := dqNormalizedName(c.a), dqNormalizedName(c.b); got != want {
			t.Errorf("normalize(%q) = %q, normalize(%q) = %q; want equal", c.a, got, c.b, want)
		}
	}
	if dqNormalizedName("Jane Doe") == dqNormalizedName("Jane Smith") {
		t.Errorf("distinct names must not collide")
	}
}

func TestDataQuality_NextBirthdayLeapDay(t *testing.T) {
	// A Feb 29 birthday falls on Feb 28 in a non-leap year.
	occ, ok := dqNextBirthday(time.Date(0, 2, 29, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatalf("expected an occurrence")
	}
	if occ.Month() != time.February || occ.Day() != 28 || occ.Year() != 2027 {
		t.Errorf("occurrence = %v, want 2027-02-28", occ)
	}
	// Today's birthday is within the window (0 days away) and rolls to next
	// year once passed.
	today := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	sameDay, ok := dqNextBirthday(time.Date(1990, 3, 1, 0, 0, 0, 0, time.UTC), today)
	if !ok || !sameDay.Equal(today) {
		t.Errorf("today's birthday = %v (%v), want %v", sameDay, ok, today)
	}
	passed, ok := dqNextBirthday(time.Date(1990, 2, 28, 0, 0, 0, 0, time.UTC), today)
	if !ok || passed.Year() != 2028 {
		t.Errorf("passed birthday = %v (%v), want 2028", passed, ok)
	}
}

func TestDataQuality_UnauthenticatedRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupDataQualityAuthRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/data-quality", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 redirect to login", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestDataQuality_ReadOnly(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()

	p, err := h.people.Create(ctx, "No Year", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := h.events.CreateForPerson(ctx, p.ID, "birthday", time.Date(0, 6, 8, 0, 0, 0, 0, time.UTC), ""); err != nil {
		t.Fatalf("create birthday: %v", err)
	}

	if code, body := getDataQualityPage(t, h); code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", code, body)
	}

	people, err := h.people.List(ctx)
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 1 {
		t.Errorf("people = %d, want 1 (page must not write)", len(people))
	}
	events, err := h.events.ListByPerson(ctx, p.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("events = %d, want 1 (page must not write)", len(events))
	}
}

// dqSection returns the HTML slice of one page section so a test can assert
// what a section lists without matching the summary or another section.
func dqSection(body, id string) string {
	start := strings.Index(body, `id="`+id+`"`)
	if start == -1 {
		return ""
	}
	rest := body[start:]
	if end := strings.Index(rest, "</section>"); end != -1 {
		return rest[:end]
	}
	return rest
}
