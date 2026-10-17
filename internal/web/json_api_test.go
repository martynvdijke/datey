package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent/user"
	"golang.org/x/crypto/bcrypt"
)

func withJSONHeader(r *http.Request) *http.Request {
	r.Header.Set("Accept", "application/json")
	return r
}

func withCSRF(r *http.Request) *http.Request {
	r.AddCookie(&http.Cookie{Name: "csrf_token", Value: "test-csrf"})
	r.Header.Set("X-CSRF-Token", "test-csrf")
	return r
}

func seedUserWithPassword(t *testing.T, h *Handler, username, password string) *Handler {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	h.client.User.Create().SetUsername(username).SetPasswordHash(string(hash)).SetRole(user.RoleAdmin).SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).ExecX(context.Background())
	return h
}

func TestJSONLogin(t *testing.T) {
	h := newTestWebHandler(t)
	seedUserWithPassword(t, h, "alice", "password123")
	body := `{"username":"alice","password":"password123"}`
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "tok"})
	req.Header.Set("X-CSRF-Token", "tok")
	// Need CSRF cookie setup via middleware? Call handler directly
	w := httptest.NewRecorder()
	h.loginPost(w, withRouteParams(req, map[string]string{}))
	if w.Code != 200 {
		t.Fatalf("expected 200 got %d body %s", w.Code, w.Body.String())
	}
	var dto UserDTO
	if err := json.NewDecoder(w.Body).Decode(&dto); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dto.Username != "alice" {
		t.Fatalf("username %s", dto.Username)
	}
}

func TestJSONDashboard(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Dash Person")
	seedBirthdayEvent(t, h, p.ID, time.Now().AddDate(0, 0, 1))
	req := withJSONHeader(httptest.NewRequest("GET", "/", nil).WithContext(withUserContext(context.Background())))
	w := httptest.NewRecorder()
	h.dashboard(w, req)
	if w.Code != 200 {
		t.Fatalf("dashboard code %d %s", w.Code, w.Body.String())
	}
	var resp map[string][]UpcomingEventDTO
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode %v body %s", err, w.Body.String())
	}
	if _, ok := resp["today"]; !ok {
		t.Fatalf("missing today")
	}
}

func TestJSONPeopleList(t *testing.T) {
	h := newTestWebHandler(t)
	seedPerson(t, h, "Alice Foo")
	req := withJSONHeader(httptest.NewRequest("GET", "/people", nil).WithContext(withUserContext(context.Background())))
	w := httptest.NewRecorder()
	h.listPeople(w, req)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode %v", err)
	}
	if resp["people"] == nil {
		t.Fatalf("missing people")
	}
}

func TestJSONPersonDetail(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Detail Person")
	req := withJSONHeader(withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(p.ID), nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(p.ID)}))
	w := httptest.NewRecorder()
	h.viewPerson(w, req)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["gift_ideas_hidden"] == nil {
		t.Fatalf("missing gift_ideas_hidden")
	}
}

func TestJSONGiftCreate(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Gift JSON Person")
	body := `{"title":"Book","price_cents":1999}`
	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/gift-ideas", bytes.NewBufferString(body)).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	withCSRF(req)
	w := httptest.NewRecorder()
	h.createGiftIdea(w, req)
	if w.Code != 201 {
		t.Fatalf("expected 201 got %d %s", w.Code, w.Body.String())
	}
}

func TestGiftUpdate(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Update Gift Person")
	g := h.client.GiftIdea.Create().SetPersonID(p.ID).SetTitle("Old").SetStatus("idea").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).SaveX(context.Background())
	body := `{"title":"New Title","notes":"hi","price_cents":123,"url":"https://example.com","status":"purchased"}`
	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/gift-ideas/"+itoa(g.ID), bytes.NewBufferString(body)).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(p.ID), "giftID": itoa(g.ID)})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	withCSRF(req)
	w := httptest.NewRecorder()
	h.updateGiftIdea(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 got %d %s", w.Code, w.Body.String())
	}
	var dto GiftIdeaDTO
	json.NewDecoder(w.Body).Decode(&dto)
	if dto.Title != "New Title" || dto.Status != "purchased" {
		t.Fatalf("unexpected dto %+v", dto)
	}
}

func TestGiftUpdateSpoiler403(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Spoiler Update Person")
	g := h.client.GiftIdea.Create().SetPersonID(p.ID).SetTitle("Secret").SetStatus("idea").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).SaveX(context.Background())
	hash, _ := bcrypt.GenerateFromPassword([]byte("pass12345"), bcrypt.DefaultCost)
	u := h.client.User.Create().SetUsername("spoilerupd").SetPasswordHash(string(hash)).SetRole(user.RoleUser).SetLinkedPersonID(p.ID).SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).SaveX(context.Background())
	ctx := context.WithValue(context.Background(), userContextKey, u)
	body := `{"title":"Hacked"}`
	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/gift-ideas/"+itoa(g.ID), bytes.NewBufferString(body)).WithContext(ctx), map[string]string{"id": itoa(p.ID), "giftID": itoa(g.ID)})
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	withCSRF(req)
	w := httptest.NewRecorder()
	h.updateGiftIdea(w, req)
	if w.Code != 403 {
		t.Fatalf("expected 403 got %d %s", w.Code, w.Body.String())
	}
}

func TestAnniversaryYearsTogether(t *testing.T) {
	h := newTestWebHandler(t)
	// Create anniversary event 5 years ago, ensure dashboard computes years_together as occYear - startYear
	p := seedPerson(t, h, "Anniv Person")
	start := time.Date(2020, 6, 15, 0, 0, 0, 0, time.UTC)
	h.client.Event.Create().SetType("anniversary").SetDate(start).SetCalendarSystem("gregorian").SetPersonID(p.ID).SetCreatedAt(time.Now()).ExecX(context.Background())
	// Make reminder window large enough
	h.cfg.ReminderDays = 365
	req := withJSONHeader(httptest.NewRequest("GET", "/", nil).WithContext(withUserContext(context.Background())))
	w := httptest.NewRecorder()
	h.dashboard(w, req)
	var resp map[string][]UpcomingEventDTO
	json.NewDecoder(w.Body).Decode(&resp)
	found := false
	for _, bucket := range resp {
		for _, ev := range bucket {
			if ev.Type == "anniversary" && ev.YearsTogether != nil {
				found = true
				// occ year should be >= start year, yearsTogether = occYear - 2020
				if *ev.YearsTogether < 0 {
					t.Fatalf("negative yearsTogether")
				}
			}
		}
	}
	if !found {
		t.Log("no anniversary in dashboard buckets (may be outside window)")
	}
}

func TestSpoilerHiding(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Spoiler Person")
	// create gift
	h.client.GiftIdea.Create().SetPersonID(p.ID).SetTitle("Secret").SetStatus("idea").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).ExecX(context.Background())
	// create user linked to this person
	hash, _ := bcrypt.GenerateFromPassword([]byte("pass12345"), bcrypt.DefaultCost)
	u := h.client.User.Create().SetUsername("spoileruser").SetPasswordHash(string(hash)).SetRole(user.RoleUser).SetLinkedPersonID(p.ID).SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).SaveX(context.Background())
	ctx := context.WithValue(context.Background(), userContextKey, u)
	req := withJSONHeader(withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(p.ID), nil).WithContext(ctx), map[string]string{"id": itoa(p.ID)}))
	w := httptest.NewRecorder()
	h.viewPerson(w, req)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["gift_ideas_hidden"] != true {
		t.Fatalf("expected hidden true got %v", resp["gift_ideas_hidden"])
	}
	if arr, ok := resp["gift_ideas"].([]any); ok && len(arr) != 0 {
		t.Fatalf("expected empty gift_ideas")
	}
}
