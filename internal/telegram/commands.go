package telegram

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/search"
)

const (
	// maxUpcomingLines bounds how many occurrences one reply renders, so a
	// large window cannot turn into a huge message or a slow query loop.
	maxUpcomingLines = 40
	// maxPersonCandidates bounds the "did you mean" list.
	maxPersonCandidates = 10
	// maxSearchGroups bounds how many entity sections /search prints.
	maxSearchGroups = 6
	// maxSearchPerGroup bounds the rows printed per section.
	maxSearchPerGroup = 5
	// maxSnippetRunes bounds one snippet line.
	maxSnippetRunes = 120
)

func (b *Bot) handleUpcoming(ctx context.Context, arg string) string {
	days := defaultUpcomingDays
	if arg != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(arg)); err == nil {
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
	to := from.AddDate(0, 0, days)
	occs, err := b.events.ListUpcomingOccurrences(ctx, from, to)
	if err != nil {
		slog.Warn("telegram: list upcoming failed", "error", err)
		return "Failed to load upcoming events."
	}
	if len(occs) == 0 {
		return fmt.Sprintf("No upcoming events in the next %d days.", days)
	}
	today := todayStartUTC()
	var sb strings.Builder
	fmt.Fprintf(&sb, "Upcoming — next %d days\n", days)
	shown := len(occs)
	if shown > maxUpcomingLines {
		shown = maxUpcomingLines
	}
	for i := 0; i < shown; i++ {
		oc := occs[i]
		sb.WriteString("\n")
		sb.WriteString(formatLine(oc.Event, oc.Date, today))
	}
	if rest := len(occs) - shown; rest > 0 {
		fmt.Fprintf(&sb, "\n…and %d more", rest)
	}
	return sb.String()
}

func (b *Bot) handleToday(ctx context.Context) string {
	from := todayStartUTC()
	endOfToday := from.AddDate(0, 0, 1).Add(-time.Nanosecond)
	occs, err := b.events.ListUpcomingOccurrences(ctx, from, endOfToday)
	if err != nil {
		slog.Warn("telegram: list today failed", "error", err)
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
	sb.WriteString("Today\n")
	for i, oc := range filtered {
		if i >= maxUpcomingLines {
			fmt.Fprintf(&sb, "\n…and %d more", len(filtered)-maxUpcomingLines)
			break
		}
		sb.WriteString("\n")
		sb.WriteString(formatLine(oc.Event, oc.Date, today))
	}
	return sb.String()
}

func (b *Bot) handlePerson(ctx context.Context, arg string) string {
	name := strings.TrimSpace(arg)
	if name == "" {
		return "Usage: /person <name>"
	}

	// Try exact match first, then a partial (case-insensitive) search.
	person, err := b.people.FindByName(ctx, name)
	if err != nil || person == nil {
		results, sErr := b.people.Search(ctx, name)
		if sErr != nil {
			slog.Warn("telegram: person search failed", "error", sErr)
			return fmt.Sprintf("Could not look up %q right now.", name)
		}
		switch len(results) {
		case 0:
			return fmt.Sprintf("No person named %q.", name)
		case 1:
			person = results[0]
		default:
			return personCandidates(results)
		}
	}

	// List upcoming occurrences for this person in the next 366 days.
	from := todayStartUTC()
	to := from.AddDate(0, 0, 366)
	occs, err := b.events.ListUpcomingOccurrences(ctx, from, to)
	if err != nil {
		slog.Warn("telegram: list person events failed", "error", err)
		return "Failed to load events."
	}
	var filtered []repository.EventOccurrence
	for _, oc := range occs {
		if strings.EqualFold(ownerName(oc.Event), person.Name) {
			filtered = append(filtered, oc)
		}
	}
	if len(filtered) == 0 {
		return fmt.Sprintf("No upcoming events for %s.", person.Name)
	}
	today := todayStartUTC()
	var sb strings.Builder
	sb.WriteString(person.Name)
	shown := len(filtered)
	if shown > maxUpcomingLines {
		shown = maxUpcomingLines
	}
	for i := 0; i < shown; i++ {
		oc := filtered[i]
		sb.WriteString("\n")
		sb.WriteString(formatLine(oc.Event, oc.Date, today))
	}
	if rest := len(filtered) - shown; rest > 0 {
		fmt.Fprintf(&sb, "\n…and %d more", rest)
	}
	return sb.String()
}

// personCandidates renders the "did you mean" reply for an ambiguous name.
func personCandidates(people []*ent.Person) string {
	var sb strings.Builder
	sb.WriteString("Did you mean:\n")
	shown := len(people)
	if shown > maxPersonCandidates {
		shown = maxPersonCandidates
	}
	for i := 0; i < shown; i++ {
		sb.WriteString("• ")
		sb.WriteString(people[i].Name)
		sb.WriteString("\n")
	}
	if rest := len(people) - shown; rest > 0 {
		fmt.Fprintf(&sb, "…and %d more\n", rest)
	}
	sb.WriteString("Please be more specific, e.g. /person First Last.")
	return sb.String()
}

func (b *Bot) handleSearch(ctx context.Context, arg string) string {
	q := strings.TrimSpace(arg)
	if q == "" {
		return "Usage: /search <query>"
	}
	if b.search == nil {
		return "Search is unavailable right now."
	}
	groups, err := b.search.Search(ctx, q)
	if err != nil {
		// Never echo the driver error: it may contain SQL internals.
		slog.Warn("telegram: search failed", "error", err)
		return "Search is unavailable right now."
	}
	groups = nonEmptyGroups(groups)
	if len(groups) == 0 {
		return fmt.Sprintf("No matches for %q.", q)
	}
	if len(groups) > maxSearchGroups {
		groups = groups[:maxSearchGroups]
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Results for %q\n", q)
	for _, g := range groups {
		sb.WriteString("\n")
		sb.WriteString(searchGroupTitle(g.Entity))
		sb.WriteString("\n")
		rows := g.Results
		if len(rows) > maxSearchPerGroup {
			rows = rows[:maxSearchPerGroup]
		}
		for _, res := range rows {
			sb.WriteString("• ")
			sb.WriteString(searchResultLabel(res))
			if snip := plainSnippet(res.Snippet); snip != "" {
				sb.WriteString("\n  ")
				sb.WriteString(snip)
			}
			sb.WriteString("\n")
		}
		if rest := len(g.Results) - len(rows); rest > 0 {
			fmt.Fprintf(&sb, "…and %d more\n", rest)
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func nonEmptyGroups(groups []search.Group) []search.Group {
	out := make([]search.Group, 0, len(groups))
	for _, g := range groups {
		if len(g.Results) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// searchGroupTitle maps an index entity to a heading.
func searchGroupTitle(e search.Entity) string {
	switch e {
	case search.EntityPerson:
		return "People"
	case search.EntityPersonNote:
		return "Notes"
	case search.EntityGiftIdea:
		return "Gift ideas"
	case search.EntityEvent:
		return "Events"
	case search.EntityGroup:
		return "Groups"
	case search.EntityGroupNote:
		return "Group notes"
	default:
		return "Results"
	}
}

// searchResultLabel combines the document title with its owner's name.
func searchResultLabel(res search.Result) string {
	title := strings.TrimSpace(res.Title)
	owner := strings.TrimSpace(res.ParentTitle)
	switch {
	case owner == "" && title == "":
		return "(untitled)"
	case owner == "":
		return title
	case title == "":
		return owner
	case strings.EqualFold(title, owner):
		return owner
	default:
		return owner + " · " + title
	}
}

// plainSnippet turns the index's HTML snippet (already escaped, matched terms
// wrapped in <mark>) into a single plain-text line for Telegram. We never send
// parse_mode, so no escaping is required on our side and user text can never
// inject markup.
func plainSnippet(s template.HTML) string {
	raw := string(s)
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, "<mark>", "")
	raw = strings.ReplaceAll(raw, "</mark>", "")
	raw = html.UnescapeString(raw)
	raw = strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(raw) > maxSnippetRunes {
		raw = string([]rune(raw)[:maxSnippetRunes]) + "…"
	}
	return raw
}
