package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datey/datey/ent/giftidea"
)

func TestFormatCents(t *testing.T) {
	tests := []struct {
		cents int
		want  string
	}{
		{0, "$0.00"},
		{5, "$0.05"},
		{99, "$0.99"},
		{100, "$1.00"},
		{1999, "$19.99"},
		{100000, "$1,000.00"},
		{123456789, "$1,234,567.89"},
		{-2500, "-$25.00"},
	}
	for _, tt := range tests {
		if got := formatCents(tt.cents); got != tt.want {
			t.Errorf("formatCents(%d) = %q, want %q", tt.cents, got, tt.want)
		}
	}
}

func giftIntPtr(v int) *int { return &v }

func TestStatsGiftSpendAggregation(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()

	alice := createTestPerson(t, h, "Alice")
	bob := createTestPerson(t, h, "Bob")

	// Alice: one purchased idea (1999) and one open idea (500).
	addTestGift(t, h, alice, giftidea.StatusPurchased, giftIntPtr(1999))
	addTestGift(t, h, alice, giftidea.StatusIdea, giftIntPtr(500))
	// Bob: one given idea (1000) and one idea without a price.
	addTestGift(t, h, bob, giftidea.StatusGiven, giftIntPtr(1000))
	addTestGift(t, h, bob, giftidea.StatusIdea, nil)

	// Sanity check the underlying rows went in as expected.
	all, err := h.giftIdeas.ListAll(ctx)
	if err != nil {
		t.Fatalf("list gift ideas: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 gift ideas, got %d", len(all))
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
		"Gift Spend",
		"$34.99", // total: 1999 + 500 + 1000
		"$19.99", // purchased status subtotal
		"$5.00",  // idea status subtotal
		"$10.00", // given status subtotal
		"$24.99", // Alice total (1999 + 500)
		"3 / 4",  // priced / all
		"Alice",
		"Bob",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("stats body missing %q", want)
		}
	}

	// Top spender list is ordered by spend, so Alice precedes Bob.
	ai := strings.Index(body, "Alice")
	bi := strings.Index(body, "Bob")
	if ai == -1 || bi == -1 || ai > bi {
		t.Errorf("expected Alice before Bob in top spenders (ai=%d bi=%d)", ai, bi)
	}

	// Gift spend is bucketed by lifecycle timestamps now, so the panel must
	// render dated buckets rather than the old "no purchase dates" notice.
	if !strings.Contains(body, `data-gift-bucket="`) {
		t.Error("expected the stats page to render gift spend buckets")
	}
}

func TestStatsGiftSpendEmptyState(t *testing.T) {
	h := newTestWebHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.statsPage(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "No gift ideas yet.") {
		t.Error("expected gift-spend empty state")
	}
}
