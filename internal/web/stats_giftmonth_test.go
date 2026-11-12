package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent/giftidea"
)

func TestStatsGiftSpendByMonth(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()

	alice := createTestPerson(t, h, "Alice")

	// One given gift dated inside the window, with both stamps present (the
	// given date must win).
	dated, err := h.giftIdeas.Create(ctx, alice, "Dated gift", "", giftIntPtr(1234), "")
	if err != nil {
		t.Fatalf("create dated gift: %v", err)
	}
	lastMonth := time.Now().AddDate(0, -1, 0)
	oldPurchase := lastMonth.AddDate(0, -1, 0)
	if _, err := h.client.GiftIdea.UpdateOneID(dated.ID).
		SetStatus(giftidea.StatusGiven).
		SetPurchasedAt(oldPurchase).
		SetGivenAt(lastMonth).
		Save(ctx); err != nil {
		t.Fatalf("stamp dated gift: %v", err)
	}

	// A priced purchase with no lifecycle timestamp must land in the explicit
	// undated bucket, not disappear.
	undated, err := h.giftIdeas.Create(ctx, alice, "Undated gift", "", giftIntPtr(500), "")
	if err != nil {
		t.Fatalf("create undated gift: %v", err)
	}
	if _, err := h.client.GiftIdea.UpdateOneID(undated.ID).
		SetStatus(giftidea.StatusPurchased).
		Save(ctx); err != nil {
		t.Fatalf("set undated status: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.statsPage(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stats page: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	for _, want := range []string{
		"$12.34", // dated gift, bucketed by month
		"$5.00",  // undated gift, still shown
		`data-gift-bucket="undated"`,
		"$17.34", // total unchanged by bucketing
	} {
		if !strings.Contains(body, want) {
			t.Errorf("stats body missing %q", want)
		}
	}

	monthRows := regexp.MustCompile(`data-gift-bucket="[A-Z][a-z]{2} \d{2}"`).FindAllString(body, -1)
	if len(monthRows) != 1 {
		t.Errorf("expected exactly one dated month bucket, got %v", monthRows)
	}

	// Existing panels keep working alongside the new one.
	for _, want := range []string{"Gift Spend", "Top spenders", "Alice"} {
		if !strings.Contains(body, want) {
			t.Errorf("stats body missing existing panel content %q", want)
		}
	}
}
