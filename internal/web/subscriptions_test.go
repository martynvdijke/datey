package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/datey/datey/internal/repository"
)

func subscriptionRequest(t *testing.T, method string, values url.Values, id int) *http.Request {
	t.Helper()
	var req *http.Request
	if values != nil {
		req = httptest.NewRequest(method, "/settings/subscriptions", strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, "/settings/subscriptions", nil)
	}
	ctx := withUserContext(req.Context())
	if id != 0 {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", itoa(id))
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}
	return req.WithContext(ctx)
}

func TestSubscriptions_ListRenders(t *testing.T) {
	h := newTestWebHandler(t)
	rec := httptest.NewRecorder()
	h.subscriptionsPage(rec, subscriptionRequest(t, http.MethodGet, nil, 0))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Calendar subscriptions") {
		t.Error("page does not contain the subscriptions heading")
	}
}

func TestSubscriptions_CreateThenList(t *testing.T) {
	h := newTestWebHandler(t)
	form := url.Values{"name": {"School"}, "url": {"https://example.com/school.ics"}, "enabled": {"on"}}
	rec := httptest.NewRecorder()
	h.subscriptionCreate(rec, subscriptionRequest(t, http.MethodPost, form, 0))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	list := httptest.NewRecorder()
	h.subscriptionsPage(list, subscriptionRequest(t, http.MethodGet, nil, 0))
	if !strings.Contains(list.Body.String(), "School") || !strings.Contains(list.Body.String(), "school.ics") {
		t.Error("created subscription missing from list")
	}
}

func TestSubscriptions_CreateRejectsBadURL(t *testing.T) {
	h := newTestWebHandler(t)
	form := url.Values{"name": {"Bad"}, "url": {"javascript:alert(1)"}}
	rec := httptest.NewRecorder()
	h.subscriptionCreate(rec, subscriptionRequest(t, http.MethodPost, form, 0))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (re-rendered form)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "absolute http") {
		t.Error("validation error not rendered")
	}
}

func TestSubscriptions_DeleteDetachesEvents(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()
	sub, err := repository.NewSubscriptionRepository(h.client).Create(ctx, "School", "https://example.com/school.ics", true)
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	ev, err := repository.NewEventRepository(h.client).CreateForSubscription(ctx, sub.ID, "uid-1", "custom",
		time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC), "")
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	rec := httptest.NewRecorder()
	h.subscriptionDelete(rec, subscriptionRequest(t, http.MethodPost, url.Values{}, sub.ID))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	got, err := repository.NewEventRepository(h.client).Get(ctx, ev.ID)
	if err != nil {
		t.Fatalf("event was deleted with its subscription: %v", err)
	}
	if got.SubscriptionID != nil {
		t.Errorf("subscription_id = %v, want nil after detach", got.SubscriptionID)
	}
	if _, err := repository.NewSubscriptionRepository(h.client).Get(ctx, sub.ID); err == nil {
		t.Error("subscription still exists after delete")
	}
}
