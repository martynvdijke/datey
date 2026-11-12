package web

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent"
)

// newRuleEvent creates an ownerless rule event, the shape the migration and the
// recurring-rules web form produce.
func newRuleEvent(t *testing.T, h *Handler, kind string, ordinal, weekday *int, date time.Time) *ent.Event {
	t.Helper()
	create := h.client.Event.Create().
		SetType("holiday").
		SetDate(date).
		SetDescription("Rule event").
		SetCalendarSystem("gregorian").
		SetFrequency("annual").
		SetCreatedAt(time.Now()).
		SetRuleKind(kind)
	if kind == "nth_weekday" {
		if ordinal != nil {
			create = create.SetRuleOrdinal(*ordinal)
		}
		if weekday != nil {
			create = create.SetRuleWeekday(*weekday)
		}
	}
	e, err := create.Save(context.Background())
	if err != nil {
		t.Fatalf("create rule event: %v", err)
	}
	return e
}

func iptr(v int) *int { return &v }

func TestICalFeedRuleEventSecondSundayOfMay(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	newRuleEvent(t, h, "nth_weekday", iptr(2), iptr(0),
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC))

	body := fetchGlobalFeed(t, h)
	if !strings.Contains(body, "DTSTART;VALUE=DATE:20260510") {
		t.Errorf("expected the rule date as DTSTART, got:\n%s", body)
	}
	if !strings.Contains(body, "RRULE:FREQ=YEARLY;BYMONTH=5;BYDAY=2SU") {
		t.Errorf("expected BYMONTH/BYDAY rule, got:\n%s", body)
	}
	if !strings.Contains(body, "SUMMARY:Rule event - holiday") {
		t.Errorf("expected the description to name the ownerless event, got:\n%s", body)
	}
}

func TestICalFeedRuleEventLastSundayOfMay(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	newRuleEvent(t, h, "nth_weekday", iptr(5), iptr(0),
		time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC))

	body := fetchGlobalFeed(t, h)
	if !strings.Contains(body, "RRULE:FREQ=YEARLY;BYMONTH=5;BYDAY=-1SU") {
		t.Errorf("expected last-Sunday BYDAY=-1SU, got:\n%s", body)
	}
	if strings.Contains(body, "RDATE") {
		t.Errorf("nth-weekday rule must not materialize RDATEs, got:\n%s", body)
	}
}

func TestICalFeedRuleEventEasterUsesRDATE(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	newRuleEvent(t, h, "easter_sunday", nil, nil,
		time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC))

	body := fetchGlobalFeed(t, h)
	if strings.Contains(body, "RRULE") {
		t.Errorf("Easter date moves yearly, must not be an RRULE, got:\n%s", body)
	}
	if !strings.Contains(body, "DTSTART;VALUE=DATE:20260405") {
		t.Errorf("expected anchor date as DTSTART, got:\n%s", body)
	}
	for _, want := range []string{"RDATE;VALUE=DATE:20270328", "RDATE;VALUE=DATE:20280416"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q, got:\n%s", want, body)
		}
	}
}
