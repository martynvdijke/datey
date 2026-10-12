package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/internal/repository"
)

func (b *Bot) handleUpcoming(ctx context.Context, arg string) string {
	days := b.cfg.ReminderDays
	if arg != "" {
		n, err := strconv.Atoi(strings.TrimSpace(arg))
		if err == nil {
			days = n
		}
	}
	if days < 1 {
		days = 1
	}
	if days > 365 {
		days = 365
	}
	from := todayStartUTC()
	to := time.Now().UTC().AddDate(0, 0, days)
	occs, err := b.events.ListUpcomingOccurrences(ctx, from, to)
	if err != nil {
		return "Failed to load upcoming events."
	}
	if len(occs) == 0 {
		return fmt.Sprintf("No upcoming events in the next %d days.", days)
	}
	today := todayStartUTC()
	var sb strings.Builder
	for i, oc := range occs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(formatLine(oc.Event, oc.Date, today))
	}
	return sb.String()
}

func (b *Bot) handleToday(ctx context.Context) string {
	from := todayStartUTC()
	endOfToday := from.AddDate(0, 0, 1).Add(-time.Nanosecond)
	occs, err := b.events.ListUpcomingOccurrences(ctx, from, endOfToday)
	if err != nil {
		return "Failed to load today's events."
	}
	// Filter to date-only equals today.
	var filtered []repository.EventOccurrence
	for _, oc := range occs {
		if oc.Date.Year() == from.Year() && oc.Date.Month() == from.Month() && oc.Date.Day() == from.Day() {
			filtered = append(filtered, oc)
		}
	}
	if len(filtered) == 0 {
		return "Nothing today. 🎉"
	}
	today := todayStartUTC()
	var sb strings.Builder
	for i, oc := range filtered {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(formatLine(oc.Event, oc.Date, today))
	}
	return sb.String()
}

func (b *Bot) handlePerson(ctx context.Context, arg string) string {
	if strings.TrimSpace(arg) == "" {
		return "Usage: /person <name>"
	}
	name := strings.TrimSpace(arg)

	// Try exact match.
	person, err := b.people.FindByName(ctx, name)
	if err != nil || person == nil {
		// Fallback to search.
		results, sErr := b.people.Search(ctx, name)
		if sErr != nil {
			return fmt.Sprintf("No person named %q.", name)
		}
		if len(results) == 0 {
			return fmt.Sprintf("No person named %q.", name)
		}
		if len(results) > 1 {
			names := make([]string, 0, len(results))
			for _, p := range results {
				names = append(names, p.Name)
			}
			return "Did you mean: " + strings.Join(names, ", ")
		}
		person = results[0]
	}

	// List upcoming occurrences for this person in next 366 days.
	from := todayStartUTC()
	to := time.Now().UTC().AddDate(0, 0, 366)
	occs, err := b.events.ListUpcomingOccurrences(ctx, from, to)
	if err != nil {
		return "Failed to load events."
	}
	var filtered []repository.EventOccurrence
	for _, oc := range occs {
		if ownerName(oc.Event) == person.Name {
			filtered = append(filtered, oc)
		}
	}
	if len(filtered) == 0 {
		return fmt.Sprintf("No upcoming events for %s.", person.Name)
	}
	today := todayStartUTC()
	var sb strings.Builder
	for i, oc := range filtered {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(formatLine(oc.Event, oc.Date, today))
	}
	return sb.String()
}

func (b *Bot) handleSearch(ctx context.Context, arg string) string {
	if strings.TrimSpace(arg) == "" {
		return "Usage: /search <query>"
	}
	q := strings.TrimSpace(arg)
	results, err := b.people.Search(ctx, q)
	if err != nil {
		return fmt.Sprintf("No people match %q.", q)
	}
	if len(results) == 0 {
		return fmt.Sprintf("No people match %q.", q)
	}
	// Limit to ~10.
	if len(results) > 10 {
		results = results[:10]
	}

	// Fetch upcoming occurrences for next 366 days to annotate.
	from := todayStartUTC()
	to := time.Now().UTC().AddDate(0, 0, 366)
	occs, _ := b.events.ListUpcomingOccurrences(ctx, from, to)
	// Build map owner -> first occurrence summary
	nextByOwner := map[string]repository.EventOccurrence{}
	for _, oc := range occs {
		on := ownerName(oc.Event)
		if _, ok := nextByOwner[on]; !ok {
			nextByOwner[on] = oc
		}
	}

	today := todayStartUTC()
	var sb strings.Builder
	for i, p := range results {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(p.Name)
		if oc, ok := nextByOwner[p.Name]; ok {
			sb.WriteString(" — " + formatDetail(oc.Event, oc.Date, today))
		}
	}
	return sb.String()
}
