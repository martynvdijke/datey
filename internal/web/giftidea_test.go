package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datey/datey/ent/giftidea"
)

func TestCreateGiftIdea_RejectsEmptyTitle(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Gift Person")
	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/gift-ideas", strings.NewReader("title=&notes=hello")), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.createGiftIdea(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected error redirect, got %q", loc)
	}
	list, _ := h.giftIdeas.ListByPerson(context.Background(), p.ID)
	if len(list) != 0 {
		t.Errorf("expected no gift created")
	}
}

func TestCreateGiftIdea_StatusTransition(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Transition Person")
	// create
	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/gift-ideas", strings.NewReader("title=Bike")), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.createGiftIdea(w, req)
	list, _ := h.giftIdeas.ListByPerson(context.Background(), p.ID)
	if len(list) != 1 {
		t.Fatalf("expected 1 gift")
	}
	gid := list[0].ID
	// update to purchased
	req = withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/gift-ideas/"+itoa(gid)+"/status", strings.NewReader("status=purchased")), map[string]string{"id": itoa(p.ID), "giftID": itoa(gid)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	h.updateGiftIdeaStatus(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect got %d", w.Code)
	}
	got, _ := h.giftIdeas.Get(context.Background(), gid)
	if got.Status != giftidea.StatusPurchased {
		t.Errorf("expected purchased, got %s", got.Status)
	}
}

func TestGiftIdeaToggleHidesPurchased(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Toggle Person")
	g1, _ := h.giftIdeas.Create(context.Background(), p.ID, "Idea", "", nil, "")
	g2, _ := h.giftIdeas.Create(context.Background(), p.ID, "PurchasedOne", "", nil, "")
	_, _ = h.giftIdeas.UpdateStatus(context.Background(), g2.ID, giftidea.StatusPurchased)
	_ = g1
	// without show_purchased
	req := withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(p.ID), nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(p.ID)})
	w := httptest.NewRecorder()
	h.viewPerson(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "Idea") {
		t.Error("expected Idea visible")
	}
	if strings.Contains(body, "PurchasedOne") {
		t.Error("purchased should be hidden by default")
	}
	// with toggle
	req = withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(p.ID)+"?show_purchased=1", nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(p.ID)})
	w = httptest.NewRecorder()
	h.viewPerson(w, req)
	body = w.Body.String()
	if !strings.Contains(body, "PurchasedOne") {
		t.Error("expected purchased visible with toggle")
	}
}

func TestDeleteGiftIdea(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Delete Person")
	g, _ := h.giftIdeas.Create(context.Background(), p.ID, "ToDelete", "", nil, "")
	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/gift-ideas/"+itoa(g.ID)+"/delete", nil), map[string]string{"id": itoa(p.ID), "giftID": itoa(g.ID)})
	w := httptest.NewRecorder()
	h.deleteGiftIdea(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect got %d", w.Code)
	}
	list, _ := h.giftIdeas.ListByPerson(context.Background(), p.ID)
	if len(list) != 0 {
		t.Errorf("expected deleted")
	}
}

func TestTemplateRenderGiftIdeas(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Render Person")
	_, _ = h.giftIdeas.Create(context.Background(), p.ID, "RenderIdea", "", nil, "")
	req := withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(p.ID), nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(p.ID)})
	w := httptest.NewRecorder()
	h.viewPerson(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "Gift Ideas") {
		t.Error("expected Gift Ideas card")
	}
	if !strings.Contains(body, "RenderIdea") {
		t.Error("expected idea title rendered")
	}
	if !strings.Contains(body, "Show purchased") {
		t.Error("expected spoiler toggle link")
	}
}
