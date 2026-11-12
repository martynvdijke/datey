// Package reminderplan validates, merges, and evaluates reminder lead-time
// stages (the "7 days before, then 1 day before" list) shared by the settings
// form, the person/event overrides, and the scheduler.
package reminderplan

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	// MinStage is the shortest allowed lead time, in days.
	MinStage = 1
	// MaxStage is the longest allowed lead time, in days.
	MaxStage = 365
	// MaxStages caps how many lead times one list may carry.
	MaxStages = 10
	// DigestHeader is the first line of a combined digest message.
	DigestHeader = "Upcoming reminders"
)

// DigestEntry is one line of a combined daily digest: a title (usually the
// contact and event type) and a message (usually the date and days remaining).
type DigestEntry struct {
	Title   string
	Message string
}

// ParseStages parses a user-supplied stage list such as "7, 1" or "7 1".
// Values may be separated by commas, whitespace, or both. Duplicates are
// collapsed and the result is sorted ascending; an empty input yields nil.
// Non-numeric values, values outside 1..365, and more than MaxStages distinct
// stages are rejected.
func ParseStages(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	stages := make([]int, 0, len(fields))
	for _, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil {
			return nil, fmt.Errorf("reminder stage %q is not a number", field)
		}
		if n < MinStage || n > MaxStage {
			return nil, fmt.Errorf("reminder stage %d must be between %d and %d", n, MinStage, MaxStage)
		}
		stages = append(stages, n)
	}
	stages = dedupeSort(stages)
	if len(stages) > MaxStages {
		return nil, fmt.Errorf("at most %d reminder stages are allowed, got %d", MaxStages, len(stages))
	}
	return stages, nil
}

// NormalizeStages returns a deduplicated, ascending copy of in with values
// outside 1..365 dropped and at most MaxStages entries kept (the most imminent
// ones). It never returns nil.
func NormalizeStages(in []int) []int {
	out := dedupeSort(in)
	if len(out) > MaxStages {
		out = out[:MaxStages]
	}
	return out
}

// dedupeSort drops out-of-range and duplicate values, then sorts ascending.
func dedupeSort(in []int) []int {
	seen := make(map[int]bool, len(in))
	out := make([]int, 0, len(in))
	for _, v := range in {
		if v < MinStage || v > MaxStage || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

// EffectiveStages resolves the stage list for one occurrence. The most
// specific non-empty list wins (event over person over global); the override
// replaces rather than merges. The returned slice is always a copy.
func EffectiveStages(global, person, event []int) []int {
	for _, candidate := range [][]int{event, person, global} {
		if len(candidate) > 0 {
			out := make([]int, len(candidate))
			copy(out, candidate)
			return out
		}
	}
	return []int{}
}

// DayDifference returns the whole-day difference between two instants,
// comparing only their calendar dates so a reminder at 23:00 and one at 00:30
// on the following day still differ by one day.
func DayDifference(now, occasion time.Time) int {
	nowDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	occasionDay := time.Date(occasion.Year(), occasion.Month(), occasion.Day(), 0, 0, 0, 0, time.UTC)
	return int(occasionDay.Sub(nowDay).Hours() / 24)
}

// Stage reports the configured stage that falls exactly today: the whole-day
// difference between occasion and now equals one of stages. A negative
// difference (occurrence already past) never matches.
func Stage(now, occasion time.Time, stages []int) (int, bool) {
	if len(stages) == 0 {
		return 0, false
	}
	days := DayDifference(now, occasion)
	for _, stage := range stages {
		if stage == days {
			return stage, true
		}
	}
	return 0, false
}

// DigestKey builds the notification-log key for one channel's daily digest.
func DigestKey(channel, dateKey string) string {
	return channel + "-" + dateKey + "-digest"
}

// ComposeDigest renders entries as one message: a header line followed by the
// entries, separated by blank lines. An empty list yields the empty string so
// callers can decide how to phrase "nothing upcoming".
func ComposeDigest(entries []DigestEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(DigestHeader)
	for _, e := range entries {
		b.WriteString("\n\n")
		switch {
		case e.Title == "":
			b.WriteString(e.Message)
		case e.Message == "":
			b.WriteString(e.Title)
		default:
			b.WriteString(e.Title + ": " + e.Message)
		}
	}
	return strings.TrimRight(b.String(), " \t\n")
}
