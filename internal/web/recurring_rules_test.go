package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func recurringRuleRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/recurring-rules/new", h.newRecurringRuleForm)
	r.Post("/recurring-rules/new", h.createRecurringRule)
	return r
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
