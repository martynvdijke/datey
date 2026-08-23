package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTagFilter_SingleAndMulti(t *testing.T) {
	h := newTestWebHandler(t)
	p1 := seedPerson(t, h, "Alice Tag")
	p2 := seedPerson(t, h, "Bob Tag")
	p3 := seedPerson(t, h, "Carol Tag")
	_ = h.tags.AddToPerson(context.Background(), p1.ID, "vip")
	_ = h.tags.AddToPerson(context.Background(), p1.ID, "camp")
	_ = h.tags.AddToPerson(context.Background(), p2.ID, "vip")
	_ = h.tags.AddToPerson(context.Background(), p3.ID, "camp")

	// single
	req := httptest.NewRequest("GET", "/people?tag=vip", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.listPeople(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "Alice Tag") || !strings.Contains(body, "Bob Tag") {
		t.Error("single tag filter should include vip persons")
	}
	if strings.Contains(body, "Carol Tag") {
		t.Error("Carol should be excluded from vip filter")
	}
	// multi AND
	req = httptest.NewRequest("GET", "/people?tag=vip,camp", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w = httptest.NewRecorder()
	h.listPeople(w, req)
	body = w.Body.String()
	if !strings.Contains(body, "Alice Tag") {
		t.Error("AND filter should include Alice")
	}
	if strings.Contains(body, "Bob Tag") || strings.Contains(body, "Carol Tag") {
		t.Error("AND filter should exclude Bob/Carol")
	}
}

func TestTagFilter_ComposesWithQAndGroup(t *testing.T) {
	h := newTestWebHandler(t)
	g := seedGroup(t, h, "MyGroup")
	a := seedPerson(t, h, "Alice Foo")
	b := seedPerson(t, h, "Bob Foo")
	_ = h.groups.AddPerson(context.Background(), g.ID, a.ID)
	_ = h.tags.AddToPerson(context.Background(), a.ID, "vip")
	_ = h.tags.AddToPerson(context.Background(), b.ID, "vip")

	// ?tag=vip&q=Alice => only Alice
	req := httptest.NewRequest("GET", "/people?tag=vip&q=Alice", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.listPeople(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "Alice Foo") {
		t.Error("expected Alice in composed filter")
	}
	if strings.Contains(body, "Bob Foo") {
		t.Error("Bob should be excluded by q filter")
	}
	// ?tag=vip&group=MyGroup => only Alice (vip+group)
	req = httptest.NewRequest("GET", "/people?tag=vip&group="+itoa(g.ID), nil)
	req = req.WithContext(withUserContext(req.Context()))
	w = httptest.NewRecorder()
	h.listPeople(w, req)
	body = w.Body.String()
	if !strings.Contains(body, "Alice Foo") {
		t.Error("expected Alice in tag+group")
	}
	if strings.Contains(body, "Bob Foo") {
		t.Error("Bob not in group, should be excluded")
	}
}

func TestAutocompleteTags(t *testing.T) {
	h := newTestWebHandler(t)
	for _, n := range []string{"vip", "vip2", "camp"} {
		if err := h.tags.AddToPerson(context.Background(), seedPerson(t, h, "P-"+n).ID, n); err != nil {
			t.Fatalf("seed tag %s: %v", n, err)
		}
	}
	req := httptest.NewRequest("GET", "/api/tags?q=vi", nil)
	w := httptest.NewRecorder()
	h.autocompleteTags(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	var names []string
	if err := json.Unmarshal(w.Body.Bytes(), &names); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 got %v", names)
	}
	// check sorted
	if names[0] != "vip" || names[1] != "vip2" {
		t.Errorf("unexpected names %v", names)
	}
}

func TestChipRendering(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Chip Person")
	_ = h.tags.AddToPerson(context.Background(), p.ID, "vip")

	// list should render chip
	req := httptest.NewRequest("GET", "/people", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.listPeople(w, req)
	if !strings.Contains(w.Body.String(), "vip") {
		t.Error("expected tag chip in people list")
	}

	// detail should render chip
	req = withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(p.ID), nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(p.ID)})
	w = httptest.NewRecorder()
	h.viewPerson(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("viewPerson status %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "vip") {
		t.Error("expected tag chip in detail")
	}
	// also check add tag form exists
	if !strings.Contains(w.Body.String(), `name="tag"`) {
		t.Error("expected add tag form")
	}
}

func TestAddRemoveTagHandlers(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Handler Person")

	// add
	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/tags", strings.NewReader("tag=vip")), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.addPersonTag(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("add tag expected 303 got %d", w.Code)
	}
	tags, _ := h.tags.ListByPerson(context.Background(), p.ID)
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag after add")
	}
	// remove
	req = withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/tags/remove", strings.NewReader("tag=vip")), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	h.removePersonTag(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("remove expected 303 got %d", w.Code)
	}
	tags, _ = h.tags.ListByPerson(context.Background(), p.ID)
	if len(tags) != 0 {
		t.Errorf("expected 0 after remove")
	}
}
