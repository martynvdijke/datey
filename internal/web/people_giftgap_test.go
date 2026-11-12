package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent/giftidea"
)

// createTestPerson inserts a person with the given display name.
func createTestPerson(t *testing.T, h *Handler, name string) int {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	p, err := h.client.Person.Create().
		SetName(name).
		SetNotes("").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person %q: %v", name, err)
	}
	return p.ID
}

// addTestEvent inserts an annually recurring event of the given type at
// `inDays` from today, anchored in 1990 so ages stay meaningful.
func addTestEvent(t *testing.T, h *Handler, personID int, eventType string, inDays int) {
	t.Helper()
	ctx := context.Background()
	occ := time.Now().AddDate(0, 0, inDays)
	date := time.Date(1990, occ.Month(), occ.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := h.client.Event.Create().
		SetType(eventType).
		SetDate(date).
		SetCreatedAt(time.Now()).
		SetPersonID(personID).
		Save(ctx); err != nil {
		t.Fatalf("create %s event: %v", eventType, err)
	}
}

// addTestGift inserts a gift idea for the person in the given status.
func addTestGift(t *testing.T, h *Handler, personID int, status giftidea.Status, priceCents *int) {
	t.Helper()
	g, err := h.giftIdeas.Create(context.Background(), personID, "Gift", "", priceCents, "")
	if err != nil {
		t.Fatalf("create gift idea: %v", err)
	}
	if status != giftidea.StatusIdea {
		if _, err := h.giftIdeas.UpdateStatus(context.Background(), g.ID, status); err != nil {
			t.Fatalf("set gift status %q: %v", status, err)
		}
	}
}

// renderPeoplePage calls the people list handler directly (bypassing router
// auth) and returns the rendered HTML.
func renderPeoplePage(t *testing.T, h *Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/people", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.listPeople(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("people page: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func giftHintHref(personID int) string {
	return `href="/people/` + itoa(personID) + `#gift-ideas"`
}

func TestPeopleGiftHint_ShownWhenCelebrationInWindowWithoutOpenIdea(t *testing.T) {
	h := newTestWebHandler(t) // ReminderDays: 7
	missing := createTestPerson(t, h, "No Idea Person")
	addTestEvent(t, h, missing, "birthday", 3)

	body := renderPeoplePage(t, h)
	if !strings.Contains(body, "No gift idea yet") {
		t.Fatalf("expected gift-gap chip, body: %s", body)
	}
	if !strings.Contains(body, giftHintHref(missing)) {
		t.Fatalf("expected chip link to %s, body: %s", giftHintHref(missing), body)
	}
}

func TestPeopleGiftHint_HiddenWhenOpenIdeaExists(t *testing.T) {
	h := newTestWebHandler(t)
	withIdea := createTestPerson(t, h, "Has Idea Person")
	addTestEvent(t, h, withIdea, "birthday", 3)
	addTestGift(t, h, withIdea, giftidea.StatusIdea, nil)

	body := renderPeoplePage(t, h)
	if strings.Contains(body, "No gift idea yet") {
		t.Fatalf("did not expect gift-gap chip when an open idea exists")
	}
}

func TestPeopleGiftHint_ClosedStatusesDoNotCountAsOpen(t *testing.T) {
	for _, status := range []giftidea.Status{
		giftidea.StatusPurchased,
		giftidea.StatusGiven,
		giftidea.StatusArchived,
	} {
		t.Run(string(status), func(t *testing.T) {
			h := newTestWebHandler(t)
			id := createTestPerson(t, h, "Closed Gift Person")
			addTestEvent(t, h, id, "birthday", 2)
			addTestGift(t, h, id, status, nil)

			body := renderPeoplePage(t, h)
			if !strings.Contains(body, "No gift idea yet") {
				t.Fatalf("expected gift-gap chip for %q-only gift ideas", status)
			}
		})
	}
}

func TestPeopleGiftHint_HiddenOutsideReminderWindow(t *testing.T) {
	h := newTestWebHandler(t) // ReminderDays: 7
	id := createTestPerson(t, h, "Far Birthday Person")
	addTestEvent(t, h, id, "birthday", 30)

	body := renderPeoplePage(t, h)
	if strings.Contains(body, "No gift idea yet") {
		t.Fatalf("did not expect gift-gap chip outside the reminder window")
	}
}

func TestPeopleGiftHint_AnniversaryCounts(t *testing.T) {
	h := newTestWebHandler(t)
	id := createTestPerson(t, h, "Anniversary Person")
	addTestEvent(t, h, id, "anniversary", 4)

	body := renderPeoplePage(t, h)
	if !strings.Contains(body, "No gift idea yet") {
		t.Fatalf("expected gift-gap chip for an upcoming anniversary")
	}
}

func TestPeopleGiftHint_NonCelebrationEventsIgnored(t *testing.T) {
	h := newTestWebHandler(t)
	id := createTestPerson(t, h, "Holiday Person")
	addTestEvent(t, h, id, "holiday", 2)

	body := renderPeoplePage(t, h)
	if strings.Contains(body, "No gift idea yet") {
		t.Fatalf("did not expect gift-gap chip for a non-celebration event")
	}
}

func TestPeopleGiftHint_NoEventsNoChip(t *testing.T) {
	h := newTestWebHandler(t)
	createTestPerson(t, h, "Eventless Person")

	body := renderPeoplePage(t, h)
	if strings.Contains(body, "No gift idea yet") {
		t.Fatalf("did not expect gift-gap chip without any event")
	}
}
