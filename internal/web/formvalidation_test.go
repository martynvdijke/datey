package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupFormValidationRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/new", h.newPersonForm)
	r.Post("/people/new", h.createPerson)
	r.Get("/people/{id}/events/new", h.newEventForm)
	r.Post("/people/{id}/events/new", h.createEvent)
	r.Get("/groups", h.listGroups)
	r.Post("/groups/create", h.createGroup)
	r.Get("/users", h.usersList)
	r.Post("/users/create", h.userCreate)
	return r
}

func TestCreatePerson_EmptyName_ShowsInlineError(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupFormValidationRouter(h)

	body := "name=&notes=some+notes"
	req := httptest.NewRequest("POST", "/people/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (form re-rendered), got %d", w.Code)
	}
	respBody := w.Body.String()
	if !strings.Contains(respBody, "Name is required") {
		t.Errorf("expected inline error 'Name is required', got: %s", respBody[:200])
	}
	// Verify notes value is preserved
	if !strings.Contains(respBody, "some notes") {
		t.Errorf("expected notes value to be preserved, got: %s", respBody[:200])
	}
}

func TestCreateEvent_EmptyFields_ShowsInlineErrors(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupFormValidationRouter(h)

	body := "type=&date=&description="
	req := httptest.NewRequest("POST", "/people/1/events/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (form re-rendered), got %d", w.Code)
	}
	respBody := w.Body.String()
	if !strings.Contains(respBody, "Event type is required") {
		t.Errorf("expected inline error 'Event type is required', got: %s", respBody[:200])
	}
	if !strings.Contains(respBody, "Date is required") {
		t.Errorf("expected inline error 'Date is required', got: %s", respBody[:200])
	}
}

func TestCreateEvent_PreservesValuesOnError(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupFormValidationRouter(h)

	body := "type=birthday&date=invalid&description=My+Birthday+Party"
	req := httptest.NewRequest("POST", "/people/1/events/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	respBody := w.Body.String()
	// Description should be preserved
	if !strings.Contains(respBody, "My Birthday Party") {
		t.Errorf("expected description value preserved, got: %s", respBody[:300])
	}
	// Type selection should be preserved
	if !strings.Contains(respBody, `value="birthday" selected`) {
		t.Errorf("expected type 'birthday' to be selected, got: %s", respBody[:300])
	}
}

func TestCreateGroup_EmptyName_ShowsInlineError(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupFormValidationRouter(h)

	body := "name=&description=test+description"
	req := httptest.NewRequest("POST", "/groups/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (form re-rendered), got %d", w.Code)
	}
	respBody := w.Body.String()
	if !strings.Contains(respBody, "Name is required") {
		t.Errorf("expected inline error 'Name is required', got: %s", respBody[:200])
	}
	// Description should be preserved
	if !strings.Contains(respBody, "test description") {
		t.Errorf("expected description value preserved, got: %s", respBody[:200])
	}
}

func TestUserCreate_EmptyUsername_ShowsInlineError(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupFormValidationRouter(h)

	body := "username=&password=short&role=user"
	req := httptest.NewRequest("POST", "/users/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (form re-rendered), got %d", w.Code)
	}
	respBody := w.Body.String()
	if !strings.Contains(respBody, "Username is required") {
		t.Errorf("expected inline error 'Username is required', got: %s", respBody[:200])
	}
	if !strings.Contains(respBody, "Password must be at least 8 characters") {
		t.Errorf("expected inline error 'Password must be at least 8 characters', got: %s", respBody[:200])
	}
	// Role should be preserved
	if !strings.Contains(respBody, `value="user" selected`) {
		t.Errorf("expected role 'user' to be selected, got: %s", respBody[:300])
	}
}
