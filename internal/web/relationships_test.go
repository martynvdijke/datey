package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelationships_AddAndRenderBothPages(t *testing.T) {
	h := newTestWebHandler(t)
	a := seedPerson(t, h, "Rel A")
	b := seedPerson(t, h, "Rel B")
	// add partner
	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(a.ID)+"/relationships", strings.NewReader("target_id="+itoa(b.ID)+"&type=partner")), map[string]string{"id": itoa(a.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.addRelationship(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect got %d body %s", w.Code, w.Body.String())
	}
	// check both pages render
	for _, id := range []int{a.ID, b.ID} {
		req2 := withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(id), nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(id)})
		w2 := httptest.NewRecorder()
		h.viewPerson(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("view %d status %d", id, w2.Code)
		}
		body := w2.Body.String()
		if !strings.Contains(body, "Related people") {
			t.Error("expected Related people card")
		}
		otherName := b.Name
		if id == b.ID {
			otherName = a.Name
		}
		if !strings.Contains(body, otherName) {
			t.Errorf("expected other name %s in body", otherName)
		}
	}
}

func TestRelationships_PickerExcludesSelfAndRemoveFlow(t *testing.T) {
	h := newTestWebHandler(t)
	a := seedPerson(t, h, "Picker A")
	b := seedPerson(t, h, "Picker B")
	_, _ = h.relationships.Add(context.Background(), a.ID, b.ID, "sibling", "")
	req := withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(a.ID), nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(a.ID)})
	w := httptest.NewRecorder()
	h.viewPerson(w, req)
	body := w.Body.String()
	// picker should not contain self as option value - count occurrences of option for self vs other
	// Simple check: the select contains b but the count of options for a should be 1 (only b)
	if strings.Contains(body, `value="`+itoa(a.ID)+`"`) {
		t.Error("picker should exclude self")
	}
	if !strings.Contains(body, `value="`+itoa(b.ID)+`"`) {
		t.Error("picker should include other person")
	}
	// remove
	entries, _ := h.relationships.ListForPerson(context.Background(), a.ID)
	if len(entries) != 1 {
		t.Fatalf("expected 1 rel")
	}
	reqDel := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(a.ID)+"/relationships/"+itoa(entries[0].ID)+"/delete", nil), map[string]string{"id": itoa(a.ID), "relID": itoa(entries[0].ID)})
	wDel := httptest.NewRecorder()
	h.removeRelationship(wDel, reqDel)
	if wDel.Code != http.StatusSeeOther {
		t.Fatalf("remove redirect %d", wDel.Code)
	}
	remaining, _ := h.relationships.ListForPerson(context.Background(), a.ID)
	if len(remaining) != 0 {
		t.Error("expected 0 after delete")
	}
}

func TestRelationships_GuardErrorsInline(t *testing.T) {
	h := newTestWebHandler(t)
	a := seedPerson(t, h, "Guard A")
	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(a.ID)+"/relationships", strings.NewReader("target_id="+itoa(a.ID)+"&type=partner")), map[string]string{"id": itoa(a.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.addRelationship(w, req)
	// should redirect with rel_error query
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect for self-link got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "rel_error") {
		t.Errorf("expected rel_error in redirect, got %s", loc)
	}
}

func TestRelationships_CustomLabelDisplay(t *testing.T) {
	h := newTestWebHandler(t)
	a := seedPerson(t, h, "Custom A")
	b := seedPerson(t, h, "Custom B")
	_, _ = h.relationships.Add(context.Background(), a.ID, b.ID, "custom", "best friend")
	req := withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(a.ID), nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(a.ID)})
	w := httptest.NewRecorder()
	h.viewPerson(w, req)
	if !strings.Contains(w.Body.String(), "best friend") {
		t.Error("expected custom label displayed")
	}
}
