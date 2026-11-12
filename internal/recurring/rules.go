package recurring

import (
	"strings"
	"time"

	"github.com/datey/datey/ent"
)

// RuleFixed is the legacy "fixed" pattern type. It is not stored on events:
// a fixed date is already fully described by the event's own date, so an event
// with an empty RuleKind is a plain dated event.
const RuleFixed = "fixed"

// RuleLastWeekday is the legacy "last weekday of month" pattern. Events store
// it as nth_weekday with ordinal 5; NormalizeRuleKind still accepts the legacy
// spelling so hand-written rows keep resolving.
const RuleLastWeekday = "last_weekday"

// IsRuleEvent reports whether the event's date is computed from a calendar rule
// rather than the stored month/day alone.
func IsRuleEvent(e *ent.Event) bool {
	return e != nil && strings.TrimSpace(e.RuleKind) != ""
}

// NormalizeRuleKind maps a stored rule kind to its canonical value. The empty
// string is returned for a plain dated event and for an unknown kind; callers
// that must reject unknown kinds should check the raw value as well.
// "last_weekday" normalises to nth_weekday because ordinal 5 already means
// "last weekday of the month".
func NormalizeRuleKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "":
		return ""
	case NthWeekdayRule, RuleLastWeekday:
		return NthWeekdayRule
	case EasterSundayRule:
		return EasterSundayRule
	case GoodFridayRule:
		return GoodFridayRule
	case EasterMondayRule:
		return EasterMondayRule
	case AshWednesdayRule:
		return AshWednesdayRule
	case PentecostRule:
		return PentecostRule
	default:
		return ""
	}
}

// RuleDateForYear resolves the event's date in the given Gregorian year from
// its rule kind. The month of nth-weekday rules comes from the stored date
// (e.Date.Month()), their ordinal from RuleOrdinal (nil or 0 means 5 = last)
// and their weekday from RuleWeekday (nil means Sunday). Easter-based kinds
// ignore the stored date. An empty rule kind falls back to the stored
// month/day. ok is false when the rule cannot be resolved for that year (an
// Easter kind without a fixed date, an impossible ordinal, or an unknown rule
// kind).
func RuleDateForYear(e *ent.Event, year int) (time.Time, bool) {
	if e == nil {
		return time.Time{}, false
	}
	raw := strings.ToLower(strings.TrimSpace(e.RuleKind))
	kind := NormalizeRuleKind(raw)
	// A non-empty kind NormalizeRuleKind does not know is unresolvable; an empty
	// rule kind (and the legacy "fixed" spelling) means a plain dated event.
	if raw != "" && raw != RuleFixed && kind == "" {
		return time.Time{}, false
	}

	month, day := 0, 0
	if !e.Date.IsZero() {
		month, day = int(e.Date.Month()), e.Date.Day()
	}
	ordinal := 5
	if e.RuleOrdinal != nil && *e.RuleOrdinal > 0 {
		ordinal = *e.RuleOrdinal
	}
	weekday := 0
	if e.RuleWeekday != nil {
		weekday = *e.RuleWeekday
	}
	return RuleEventDate(kind, ordinal, weekday, month, day, year)
}

// RuleEventDate is the ent-free form of RuleDateForYear, usable by the web form
// before an event exists. kind may be "" (or "fixed") for a plain month/day
// date; ordinal 0 means 5 (last weekday); weekday uses Sunday=0. It returns
// ok=false for an unknown kind, an invalid month/day, or a month that has no
// such date in the given year.
func RuleEventDate(kind string, ordinal, weekday, month, day, year int) (time.Time, bool) {
	raw := strings.ToLower(strings.TrimSpace(kind))
	k := NormalizeRuleKind(raw)
	// Reject a non-empty kind that is neither a plain fixed date nor a known
	// rule; silently falling back to the stored date would hide a typo.
	if raw != "" && raw != RuleFixed && k == "" {
		return time.Time{}, false
	}
	switch {
	case k == "":
		if month < 1 || month > 12 || day < 1 || day > daysInMonthInt(time.Month(month), year) {
			return time.Time{}, false
		}
		return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), true
	case IsEasterBased(k):
		d := DateForType(k, year)
		return d, !d.IsZero()
	case k == NthWeekdayRule:
		if ordinal == 0 {
			ordinal = 5
		}
		return NthWeekdayOfMonth(year, month, ordinal, weekday)
	default:
		return time.Time{}, false
	}
}
