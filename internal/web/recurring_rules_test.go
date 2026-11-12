package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/event"
	"github.com/datey/datey/internal/recurring"
	"github.com/go-chi/chi/v5"
)

func recurringRuleRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/recurring-rules", h.listRecurringRules)
	r.Get("/recurring-rules/new", h.newRecurringRuleForm)
	r.Post("/recurring-rules/new", h.createRecurringRule)
	r.Post("/recurring-rules/{id}/delete", h.deleteRecurringRule)
	return r
}

func ruleEvents(t *testing.T, h *Handler) []*ent.Event {
	t.Helper()
	events, err := h.client.Event.Query().
		Where(event.RuleKindNEQ("")).
		All(context.Background())
	if err != nil {
		t.Fatalf("query rule events: %v", err)
	}
	return events
}

func TestRecurringRuleForm_ConditionalFields(t *testing.T) {
	h := newTestWebHandler(t)
	req := httptest.NewRequest("GET", "/recurring-rules/new", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	recurringRuleRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `value="nth_weekday"`) {
		t.Error("expected nth_weekday option in form")
	}
	if !strings.Contains(body, `>Last<`) {
		t.Error("expected Last label for ordinal 5")
	}
	if !strings.Contains(body, `id="nth-fields"`) {
		t.Error("expected nth-fields container")
	}
	if !strings.Contains(body, `id="weekday"`) {
		t.Error("expected weekday select")
	}
}

func TestRecurringRuleCreate_InvalidOrdinal(t *testing.T) {
	h := newTestWebHandler(t)
	router := recurringRuleRouter(h)
	body := "name=Test&pattern_type=nth_weekday&month=5&nth=6&weekday=0&csrf_token=x"
	req := httptest.NewRequest("POST", "/recurring-rules/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Ordinal must be 1-5") {
		t.Error("expected ordinal validation error")
	}
}

func TestRecurringRuleCreate_MissingMonth(t *testing.T) {
	h := newTestWebHandler(t)
	router := recurringRuleRouter(h)
	body := "name=Test&pattern_type=nth_weekday&nth=2&weekday=0&csrf_token=x"
	req := httptest.NewRequest("POST", "/recurring-rules/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "Month is required") {
		t.Error("expected month required error")
	}
}

func TestRecurringRuleCreate_Valid(t *testing.T) {
	h := newTestWebHandler(t)
	router := recurringRuleRouter(h)
	body := "name=MothersDay&pattern_type=nth_weekday&month=5&nth=2&weekday=0&csrf_token=x"
	req := httptest.NewRequest("POST", "/recurring-rules/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect got %d body %s", w.Code, w.Body.String())
	}
}

func postRecurringRule(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/recurring-rules/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	recurringRuleRouter(h).ServeHTTP(w, req)
	return w
}

func TestRecurringRuleCreate_CreatesRuleEvent(t *testing.T) {
	h := newTestWebHandler(t)
	w := postRecurringRule(t, h,
		"name=MothersDay&pattern_type=nth_weekday&month=5&nth=2&weekday=0&csrf_token=x")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect got %d body %s", w.Code, w.Body.String())
	}

	events := ruleEvents(t, h)
	if len(events) != 1 {
		t.Fatalf("expected one rule event, got %d", len(events))
	}
	e := events[0]
	if e.Description != "MothersDay" || e.Type != "holiday" {
		t.Errorf("unexpected event: %+v", e)
	}
	if e.RuleKind != recurring.NthWeekdayRule || e.RuleOrdinal == nil || *e.RuleOrdinal != 2 ||
		e.RuleWeekday == nil || *e.RuleWeekday != 0 {
		t.Errorf("unexpected rule fields: kind=%q ordinal=%v weekday=%v", e.RuleKind, e.RuleOrdinal, e.RuleWeekday)
	}
	if e.Frequency != recurring.FrequencyAnnual || e.CalendarSystem != "gregorian" {
		t.Errorf("rule event must be an annual gregorian event: %+v", e)
	}
	want, ok := recurring.RuleEventDate(recurring.NthWeekdayRule, 2, 0, 5, 0, time.Now().Year())
	if !ok || !e.Date.Equal(want) {
		t.Errorf("date = %s, want %s", e.Date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestRecurringRuleList_ShowsRuleEvents(t *testing.T) {
	h := newTestWebHandler(t)
	postRecurringRule(t, h, "name=MothersDay&pattern_type=nth_weekday&month=5&nth=2&weekday=0")

	req := httptest.NewRequest("GET", "/recurring-rules", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	recurringRuleRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "MothersDay") || !strings.Contains(body, "nth_weekday") {
		t.Errorf("expected the rule event in the list, got: %s", body)
	}
}

func TestRecurringRuleDelete_RemovesRuleEvent(t *testing.T) {
	h := newTestWebHandler(t)
	postRecurringRule(t, h, "name=MothersDay&pattern_type=nth_weekday&month=5&nth=2&weekday=0")
	events := ruleEvents(t, h)
	if len(events) != 1 {
		t.Fatalf("expected one rule event, got %d", len(events))
	}
	id := events[0].ID

	req := httptest.NewRequest("POST", "/recurring-rules/"+strconv.Itoa(id)+"/delete", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	recurringRuleRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body %s", w.Code, w.Body.String())
	}
	if w.Header().Get("HX-Refresh") != "true" {
		t.Error("expected HX-Refresh header")
	}
	if got := ruleEvents(t, h); len(got) != 0 {
		t.Errorf("rule event still present after delete: %v", got)
	}
}

func TestRecurringRuleDelete_RejectsNonRuleEvent(t *testing.T) {
	h := newTestWebHandler(t)
	personID := newTestPerson(t, h, "Anna")
	newTestEvent(t, h, personID, "birthday", time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	personEvents, err := h.events.ListByPerson(context.Background(), personID)
	if err != nil || len(personEvents) != 1 {
		t.Fatalf("list person events: %v (%d)", err, len(personEvents))
	}

	req := httptest.NewRequest("POST", "/recurring-rules/"+strconv.Itoa(personEvents[0].ID)+"/delete", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	recurringRuleRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a non-rule event, got %d", w.Code)
	}
}
