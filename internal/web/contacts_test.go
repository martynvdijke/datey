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

func setupContactsRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/contacts", h.listContacts)
	r.Get("/contacts/new", h.newContactForm)
	r.Post("/contacts/new", h.createContact)
	r.Get("/contacts/{id}", h.viewContact)
	r.Post("/contacts/{id}/delete", h.deleteContact)
	return r
}

func TestListContacts_Empty(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupContactsRouter(h)

	req := httptest.NewRequest("GET", "/contacts", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Contacts") {
		t.Errorf("expected Contacts in body, got: %s", w.Body.String()[:200])
	}
}

func TestNewContactForm(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupContactsRouter(h)

	req := httptest.NewRequest("GET", "/contacts/new", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Add Contact") {
		t.Errorf("expected Add Contact form, got: %s", w.Body.String()[:200])
	}
}

func TestCreateContact_Success(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupContactsRouter(h)

	body := "name=Test+Contact&notes=Hello+World"
	req := httptest.NewRequest("POST", "/contacts/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/contacts" {
		t.Errorf("expected redirect to /contacts, got %s", loc)
	}
}

func TestCreateContact_EmptyName(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupContactsRouter(h)

	body := "name=&notes=Hello"
	req := httptest.NewRequest("POST", "/contacts/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Ent schema enforces name min length, so this returns 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 due to validation, got %d", w.Code)
	}
}

func TestViewContact_NotFound(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupContactsRouter(h)

	req := httptest.NewRequest("GET", "/contacts/99999", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestViewContact_Success(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupContactsRouter(h)

	// Create a contact first
	contact, err := h.client.Contact.Create().
		SetName("View Test").
		SetNotes("test notes").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	req := httptest.NewRequest("GET", "/contacts/"+itoa(contact.ID), nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String()[:200])
	}
	if !strings.Contains(w.Body.String(), "View Test") {
		t.Errorf("expected contact name in body, got: %s", w.Body.String()[:300])
	}
}

func TestDeleteContact(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupContactsRouter(h)

	// Create a contact to delete
	contact, err := h.client.Contact.Create().
		SetName("Delete Me").
		SetNotes("to be deleted").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	req := httptest.NewRequest("POST", "/contacts/"+itoa(contact.ID)+"/delete", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}

	// Verify it's gone
	_, err = h.client.Contact.Get(context.Background(), contact.ID)
	if err == nil {
		t.Errorf("contact should have been deleted")
	}
}
