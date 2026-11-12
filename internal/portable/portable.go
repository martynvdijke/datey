// Package portable owns the human-readable, versioned JSON representation of a
// Datey dataset: the export envelope, its validation, an additive import and
// the preview that tells a user what an import would create or skip.
//
// The envelope deliberately carries only natural, human-editable fields. It
// never contains credentials or internal identifiers: no password hashes, API
// tokens, sessions, push subscriptions, notification channels, CardDAV/Google
// identifiers, Immich asset identifiers or raw vCard payloads.
package portable

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/giftidea"
	"github.com/datey/datey/internal/recurring"
	"github.com/datey/datey/internal/repository"
)

const (
	// Format is the value of the envelope's "format" field. It identifies a
	// Datey portable document and rejects unrelated JSON.
	Format = "datey-portable"
	// Version is the schema version this build reads and writes. A newer
	// version is rejected rather than guessed at.
	Version = 1
	// dateLayout is the human-editable representation of a date.
	dateLayout = "2006-01-02"
)

// Source records where an export came from. Both fields may be blank.
type Source struct {
	AppURL     string `json:"app_url,omitempty"`
	AppVersion string `json:"app_version,omitempty"`
}

// Envelope is the whole portable document.
type Envelope struct {
	Format         string          `json:"format"`
	Version        int             `json:"version"`
	ExportedAt     time.Time       `json:"exported_at"`
	Source         Source          `json:"source"`
	People         []Person        `json:"people"`
	Groups         []Group         `json:"groups"`
	Events         []Event         `json:"events"`
	GiftIdeas      []GiftIdea      `json:"gift_ideas"`
	Tags           []Tag           `json:"tags"`
	Relationships  []Relationship  `json:"relationships"`
	PersonNotes    []PersonNote    `json:"person_notes"`
	GroupNotes     []GroupNote     `json:"group_notes"`
	RecurringRules []RecurringRule `json:"recurring_rules"`
}

// Person is a contact. Group and tag membership are carried as names so the
// document stays readable and resolvable without internal IDs.
type Person struct {
	Name       string   `json:"name"`
	FirstName  string   `json:"first_name,omitempty"`
	MiddleName string   `json:"middle_name,omitempty"`
	LastName   string   `json:"last_name,omitempty"`
	Notes      string   `json:"notes,omitempty"`
	Groups     []string `json:"groups,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

// Group is a named collection of people.
type Group struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Event is a dated event owned by one person or one group.
type Event struct {
	Person          string `json:"person,omitempty"`
	Group           string `json:"group,omitempty"`
	Type            string `json:"type"`
	Date            string `json:"date"`
	Description     string `json:"description,omitempty"`
	Notes           string `json:"notes,omitempty"`
	CalendarSystem  string `json:"calendar_system,omitempty"`
	LunarMonth      *int   `json:"lunar_month,omitempty"`
	LunarDay        *int   `json:"lunar_day,omitempty"`
	LunarLeap       bool   `json:"lunar_leap,omitempty"`
	Frequency       string `json:"frequency,omitempty"`
	Interval        int    `json:"interval,omitempty"`
	EndDate         string `json:"end_date,omitempty"`
	OccurrenceCount *int   `json:"occurrence_count,omitempty"`
}

// GiftIdea is a gift attached to a person.
type GiftIdea struct {
	Person     string `json:"person"`
	Title      string `json:"title"`
	Notes      string `json:"notes,omitempty"`
	PriceCents *int   `json:"price_cents,omitempty"`
	URL        string `json:"url,omitempty"`
	Status     string `json:"status,omitempty"`
}

// Tag is a person label.
type Tag struct {
	Name string `json:"name"`
}

// Relationship is a typed link between two people, stored from "from" to "to".
type Relationship struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
}

// PersonNote is a dated note on a person's timeline.
type PersonNote struct {
	Person string `json:"person"`
	Note   string `json:"note"`
	Date   string `json:"date,omitempty"`
}

// GroupNote is a dated note on a group's timeline.
type GroupNote struct {
	Group string `json:"group"`
	Note  string `json:"note"`
	Date  string `json:"date,omitempty"`
}

// RecurringRule is a named calendar rule.
type RecurringRule struct {
	Name        string `json:"name"`
	PatternType string `json:"pattern_type"`
	Nth         int    `json:"nth,omitempty"`
	Weekday     int    `json:"weekday,omitempty"`
	Month       int    `json:"month,omitempty"`
	Day         int    `json:"day,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// Deps is the set of repositories the portable package reads and writes
// through. The web layer fills it from the handler's existing repositories, so
// no extra wiring or ent client of its own is needed.
type Deps struct {
	People        *repository.PersonRepository
	Groups        *repository.GroupRepository
	Events        *repository.EventRepository
	GiftIdeas     *repository.GiftIdeaRepository
	Tags          *repository.TagRepository
	Relationships *repository.RelationshipRepository
	PersonNotes   *repository.PersonNoteRepository
	GroupNotes    *repository.GroupNoteRepository
	Rules         *repository.RecurringRuleRepository

	Source Source
	Now    func() time.Time
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}

// ValidationError lists every problem found in a document so the import page
// can show them all at once instead of failing on the first one.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	if len(e.Problems) == 1 {
		return e.Problems[0]
	}
	return fmt.Sprintf("%d problems: %s", len(e.Problems), strings.Join(e.Problems, "; "))
}

var (
	eventTypeSet = map[string]bool{
		"birthday":    true,
		"anniversary": true,
		"wedding":     true,
		"memorial":    true,
		"custom":      true,
	}
	calendarSystemSet = map[string]bool{
		"gregorian": true,
		"lunar":     true,
		"hebrew":    true,
		"hijri":     true,
	}
	giftStatusSet = map[string]bool{
		"idea":      true,
		"purchased": true,
		"given":     true,
		"archived":  true,
	}
	relationshipTypeSet = map[string]bool{
		"partner": true,
		"parent":  true,
		"sibling": true,
		"custom":  true,
	}
)

// Parse unmarshals and validates a portable document. Malformed JSON, an
// unsupported format or a newer schema version all produce a readable error.
func Parse(data []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("not a valid Datey portable JSON document: %w", err)
	}
	if err := Validate(env); err != nil {
		return env, err
	}
	return env, nil
}

// Validate checks the document's identity and internal references. Referenced
// people, groups and tags must be present in the same envelope, because the
// document is meant to stand alone.
func Validate(env Envelope) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if env.Format != Format {
		add("unsupported format %q (want %q)", env.Format, Format)
	}
	if env.Version != Version {
		add("unsupported version %d (this build supports version %d)", env.Version, Version)
	}
	if env.Format != Format || env.Version != Version {
		return &ValidationError{Problems: problems}
	}

	people := map[string]string{}
	for i, p := range env.People {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			add("people[%d]: name is required", i)
			continue
		}
		key := nameKey(name)
		if _, dup := people[key]; dup {
			add("people[%d]: duplicate name %q", i, name)
			continue
		}
		people[key] = name
	}
	groups := map[string]string{}
	for i, g := range env.Groups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			add("groups[%d]: name is required", i)
			continue
		}
		key := nameKey(name)
		if _, dup := groups[key]; dup {
			add("groups[%d]: duplicate name %q", i, name)
			continue
		}
		groups[key] = name
	}
	tags := map[string]bool{}
	for i, t := range env.Tags {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			add("tags[%d]: name is required", i)
			continue
		}
		key := nameKey(name)
		if tags[key] {
			add("tags[%d]: duplicate name %q", i, name)
			continue
		}
		tags[key] = true
	}

	for i, p := range env.People {
		for _, g := range p.Groups {
			if _, ok := groups[nameKey(g)]; !ok {
				add("people[%d] %q: unknown group %q", i, p.Name, g)
			}
		}
		for _, t := range p.Tags {
			if !tags[nameKey(t)] {
				add("people[%d] %q: unknown tag %q", i, p.Name, t)
			}
		}
	}

	for i, e := range env.Events {
		owner := strings.TrimSpace(e.Person) != ""
		grouped := strings.TrimSpace(e.Group) != ""
		switch {
		case owner && grouped:
			add("events[%d]: an event has exactly one owner, got both person and group", i)
		case !owner && !grouped:
			add("events[%d]: an event needs a person or group owner", i)
		case owner:
			if _, ok := people[nameKey(e.Person)]; !ok {
				add("events[%d]: unknown person %q", i, e.Person)
			}
		default:
			if _, ok := groups[nameKey(e.Group)]; !ok {
				add("events[%d]: unknown group %q", i, e.Group)
			}
		}
		if strings.TrimSpace(e.Type) == "" {
			add("events[%d]: type is required", i)
		} else if !eventTypeSet[e.Type] {
			add("events[%d]: unsupported type %q", i, e.Type)
		}
		date, dateErr := parseDate(e.Date)
		if dateErr != nil {
			add("events[%d]: invalid date %q (want YYYY-MM-DD)", i, e.Date)
		}
		system := e.CalendarSystem
		if system == "" {
			system = "gregorian"
		}
		if !calendarSystemSet[system] {
			add("events[%d]: unsupported calendar system %q", i, e.CalendarSystem)
		}
		if system != "gregorian" {
			if e.LunarMonth == nil {
				add("events[%d]: lunar_month is required for %s events", i, system)
			} else if max := 12; system == "hebrew" {
				max = 13
				if *e.LunarMonth < 1 || *e.LunarMonth > max {
					add("events[%d]: lunar_month out of range for %s", i, system)
				}
			} else if *e.LunarMonth < 1 || *e.LunarMonth > max {
				add("events[%d]: lunar_month out of range", i)
			}
			if e.LunarDay == nil {
				add("events[%d]: lunar_day is required for %s events", i, system)
			} else if *e.LunarDay < 1 || *e.LunarDay > 30 {
				add("events[%d]: lunar_day out of range", i)
			}
		}
		if f := e.Frequency; f != "" {
			if _, ok := recurring.NormalizeFrequency(f); !ok {
				add("events[%d]: unsupported frequency %q", i, f)
			}
		}
		if e.Interval < 0 || e.Interval > 120 {
			add("events[%d]: interval out of range", i)
		}
		if e.OccurrenceCount != nil && *e.OccurrenceCount < 1 {
			add("events[%d]: occurrence_count must be at least 1", i)
		}
		if e.EndDate != "" {
			if end, err := parseDate(e.EndDate); err != nil {
				add("events[%d]: invalid end_date %q", i, e.EndDate)
			} else if dateErr == nil && end.Before(date) {
				add("events[%d]: end_date is before date", i)
			}
		}
	}

	for i, g := range env.GiftIdeas {
		if _, ok := people[nameKey(g.Person)]; !ok {
			add("gift_ideas[%d]: unknown person %q", i, g.Person)
		}
		if strings.TrimSpace(g.Title) == "" {
			add("gift_ideas[%d]: title is required", i)
		}
		if g.Status != "" && !giftStatusSet[g.Status] {
			add("gift_ideas[%d]: unsupported status %q", i, g.Status)
		}
		if g.PriceCents != nil && *g.PriceCents < 0 {
			add("gift_ideas[%d]: price_cents must not be negative", i)
		}
	}

	for i, r := range env.Relationships {
		from := nameKey(r.From)
		to := nameKey(r.To)
		if _, ok := people[from]; !ok {
			add("relationships[%d]: unknown person %q", i, r.From)
		}
		if _, ok := people[to]; !ok {
			add("relationships[%d]: unknown person %q", i, r.To)
		}
		if from == to && from != "" {
			add("relationships[%d]: a person cannot relate to themselves", i)
		}
		if !relationshipTypeSet[r.Type] {
			add("relationships[%d]: unsupported type %q", i, r.Type)
		}
	}

	for i, n := range env.PersonNotes {
		if _, ok := people[nameKey(n.Person)]; !ok {
			add("person_notes[%d]: unknown person %q", i, n.Person)
		}
		if strings.TrimSpace(n.Note) == "" {
			add("person_notes[%d]: note is required", i)
		}
		if n.Date != "" {
			if _, err := parseDate(n.Date); err != nil {
				add("person_notes[%d]: invalid date %q", i, n.Date)
			}
		}
	}
	for i, n := range env.GroupNotes {
		if _, ok := groups[nameKey(n.Group)]; !ok {
			add("group_notes[%d]: unknown group %q", i, n.Group)
		}
		if strings.TrimSpace(n.Note) == "" {
			add("group_notes[%d]: note is required", i)
		}
		if n.Date != "" {
			if _, err := parseDate(n.Date); err != nil {
				add("group_notes[%d]: invalid date %q", i, n.Date)
			}
		}
	}

	for i, r := range env.RecurringRules {
		if strings.TrimSpace(r.Name) == "" {
			add("recurring_rules[%d]: name is required", i)
		}
		if strings.TrimSpace(r.PatternType) == "" {
			add("recurring_rules[%d]: pattern_type is required", i)
		}
		if r.Weekday < 0 || r.Weekday > 6 {
			add("recurring_rules[%d]: weekday out of range", i)
		}
		if r.Month < 0 || r.Month > 13 {
			add("recurring_rules[%d]: month out of range", i)
		}
		if r.Day < 0 || r.Day > 31 {
			add("recurring_rules[%d]: day out of range", i)
		}
	}

	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

// Existing is a snapshot of the current library keyed by natural keys. Preview
// and Import both build it once so neither issues a query per record.
type Existing struct {
	People        map[string]Person
	PersonIDs     map[string]int
	Groups        map[string]Group
	GroupIDs      map[string]int
	Tags          map[string]bool
	Events        map[string]Event
	GiftIdeas     map[string]GiftIdea
	Relationships map[string]Relationship
	PersonNotes   map[string]PersonNote
	GroupNotes    map[string]GroupNote
	Rules         map[string]RecurringRule
}

// Counts is the new/skip/conflict breakdown for one entity type.
type Counts struct {
	New      int
	Skip     int
	Conflict int
}

// PreviewResult is the per-entity summary shown before an import is confirmed.
type PreviewResult struct {
	People         Counts
	Groups         Counts
	Tags           Counts
	Events         Counts
	GiftIdeas      Counts
	Relationships  Counts
	PersonNotes    Counts
	GroupNotes     Counts
	RecurringRules Counts
}

// EntityResult is the created/skipped outcome for one entity type.
type EntityResult struct {
	Created int
	Skipped int
}

// SkipRecord explains why one record was not imported.
type SkipRecord struct {
	Entity string
	Name   string
	Reason string
}

// Result is the summary shown after an import.
type Result struct {
	People         EntityResult
	Groups         EntityResult
	Tags           EntityResult
	Events         EntityResult
	GiftIdeas      EntityResult
	Relationships  EntityResult
	PersonNotes    EntityResult
	GroupNotes     EntityResult
	RecurringRules EntityResult

	Skipped []SkipRecord
}

// Options is reserved for future import modes. The v1 import is always
// additive and idempotent.
type Options struct{}

// Export reads every portable entity through the repositories in a stable
// order. Each repository is queried once; nothing is read from secrets or
// credential tables.
func Export(ctx context.Context, deps Deps) (Envelope, error) {
	env := Envelope{
		Format:         Format,
		Version:        Version,
		ExportedAt:     deps.now(),
		Source:         deps.Source,
		People:         []Person{},
		Groups:         []Group{},
		Events:         []Event{},
		GiftIdeas:      []GiftIdea{},
		Tags:           []Tag{},
		Relationships:  []Relationship{},
		PersonNotes:    []PersonNote{},
		GroupNotes:     []GroupNote{},
		RecurringRules: []RecurringRule{},
	}

	people, err := deps.People.List(ctx)
	if err != nil {
		return env, err
	}
	names := make(map[int]string, len(people))
	for _, p := range people {
		names[p.ID] = p.Name
		env.People = append(env.People, exportPerson(p))
	}
	sort.Slice(env.People, func(i, j int) bool {
		return nameKey(env.People[i].Name) < nameKey(env.People[j].Name)
	})

	groups, err := deps.Groups.List(ctx)
	if err != nil {
		return env, err
	}
	for _, g := range groups {
		env.Groups = append(env.Groups, Group{Name: g.Name, Description: g.Description})
	}
	sort.Slice(env.Groups, func(i, j int) bool {
		return nameKey(env.Groups[i].Name) < nameKey(env.Groups[j].Name)
	})

	tags, err := deps.Tags.ListAll(ctx)
	if err != nil {
		return env, err
	}
	for _, t := range tags {
		env.Tags = append(env.Tags, Tag{Name: t.Name})
	}
	sort.Slice(env.Tags, func(i, j int) bool { return env.Tags[i].Name < env.Tags[j].Name })

	events, err := deps.Events.List(ctx)
	if err != nil {
		return env, err
	}
	for _, e := range events {
		// Events owned by a legacy contact have no portable owner name and are
		// left out rather than exported as dangling references.
		switch {
		case e.Edges.Person != nil:
			env.Events = append(env.Events, exportEvent(e, e.Edges.Person.Name, ""))
		case e.Edges.Group != nil:
			env.Events = append(env.Events, exportEvent(e, "", e.Edges.Group.Name))
		}
	}
	sort.Slice(env.Events, func(i, j int) bool {
		a, b := env.Events[i], env.Events[j]
		if ka, kb := eventKey(a.Person, a.Group, a.Type, a.Date), eventKey(b.Person, b.Group, b.Type, b.Date); ka != kb {
			return ka < kb
		}
		return a.Description < b.Description
	})

	gifts, err := deps.GiftIdeas.ListAll(ctx)
	if err != nil {
		return env, err
	}
	for _, g := range gifts {
		if g.Edges.Person == nil {
			continue
		}
		env.GiftIdeas = append(env.GiftIdeas, GiftIdea{
			Person:     g.Edges.Person.Name,
			Title:      g.Title,
			Notes:      g.Notes,
			PriceCents: g.PriceCents,
			URL:        g.URL,
			Status:     string(g.Status),
		})
	}
	sort.Slice(env.GiftIdeas, func(i, j int) bool {
		ka := giftKey(env.GiftIdeas[i].Person, env.GiftIdeas[i].Title)
		kb := giftKey(env.GiftIdeas[j].Person, env.GiftIdeas[j].Title)
		if ka != kb {
			return ka < kb
		}
		return env.GiftIdeas[i].URL < env.GiftIdeas[j].URL
	})

	rels, err := deps.Relationships.ListAll(ctx)
	if err != nil {
		return env, err
	}
	for _, rel := range rels {
		from, okFrom := names[rel.FromID]
		to, okTo := names[rel.ToID]
		if !okFrom || !okTo {
			continue
		}
		out := Relationship{From: from, To: to, Type: string(rel.Type)}
		if rel.Label != nil {
			out.Label = *rel.Label
		}
		env.Relationships = append(env.Relationships, out)
	}
	sort.Slice(env.Relationships, func(i, j int) bool {
		return relationshipKey(env.Relationships[i].From, env.Relationships[i].To, env.Relationships[i].Type) <
			relationshipKey(env.Relationships[j].From, env.Relationships[j].To, env.Relationships[j].Type)
	})

	personNotes, err := deps.PersonNotes.ListAll(ctx)
	if err != nil {
		return env, err
	}
	for _, n := range personNotes {
		if n.Edges.Person == nil {
			continue
		}
		env.PersonNotes = append(env.PersonNotes, PersonNote{
			Person: n.Edges.Person.Name,
			Note:   n.Note,
			Date:   formatDate(n.NoteDate),
		})
	}
	sort.Slice(env.PersonNotes, func(i, j int) bool {
		a, b := env.PersonNotes[i], env.PersonNotes[j]
		ka, kb := personNoteKey(a.Person, a.Note), personNoteKey(b.Person, b.Note)
		if ka != kb {
			return ka < kb
		}
		return a.Date < b.Date
	})

	groupNotes, err := deps.GroupNotes.ListAll(ctx)
	if err != nil {
		return env, err
	}
	for _, n := range groupNotes {
		if n.Edges.Group == nil {
			continue
		}
		env.GroupNotes = append(env.GroupNotes, GroupNote{
			Group: n.Edges.Group.Name,
			Note:  n.Note,
			Date:  formatDate(n.NoteDate),
		})
	}
	sort.Slice(env.GroupNotes, func(i, j int) bool {
		a, b := env.GroupNotes[i], env.GroupNotes[j]
		ka, kb := groupNoteKey(a.Group, a.Note), groupNoteKey(b.Group, b.Note)
		if ka != kb {
			return ka < kb
		}
		return a.Date < b.Date
	})

	rules, err := deps.Rules.ListAll(ctx)
	if err != nil {
		return env, err
	}
	for _, r := range rules {
		env.RecurringRules = append(env.RecurringRules, RecurringRule{
			Name:        r.Name,
			PatternType: r.PatternType,
			Nth:         r.Nth,
			Weekday:     r.Weekday,
			Month:       r.Month,
			Day:         r.Day,
			Enabled:     r.Enabled,
		})
	}
	sort.Slice(env.RecurringRules, func(i, j int) bool {
		return nameKey(env.RecurringRules[i].Name) < nameKey(env.RecurringRules[j].Name)
	})

	return env, nil
}

// LoadExisting snapshots the current library keyed by the same natural keys
// Preview and Import use.
func LoadExisting(ctx context.Context, deps Deps) (Existing, error) {
	ex := Existing{
		People:        map[string]Person{},
		PersonIDs:     map[string]int{},
		Groups:        map[string]Group{},
		GroupIDs:      map[string]int{},
		Tags:          map[string]bool{},
		Events:        map[string]Event{},
		GiftIdeas:     map[string]GiftIdea{},
		Relationships: map[string]Relationship{},
		PersonNotes:   map[string]PersonNote{},
		GroupNotes:    map[string]GroupNote{},
		Rules:         map[string]RecurringRule{},
	}

	people, err := deps.People.List(ctx)
	if err != nil {
		return ex, err
	}
	names := make(map[int]string, len(people))
	for _, p := range people {
		key := nameKey(p.Name)
		ex.People[key] = exportPerson(p)
		ex.PersonIDs[key] = p.ID
		names[p.ID] = p.Name
	}

	groups, err := deps.Groups.List(ctx)
	if err != nil {
		return ex, err
	}
	for _, g := range groups {
		key := nameKey(g.Name)
		ex.Groups[key] = Group{Name: g.Name, Description: g.Description}
		ex.GroupIDs[key] = g.ID
	}

	tags, err := deps.Tags.ListAll(ctx)
	if err != nil {
		return ex, err
	}
	for _, t := range tags {
		ex.Tags[nameKey(t.Name)] = true
	}

	events, err := deps.Events.List(ctx)
	if err != nil {
		return ex, err
	}
	for _, e := range events {
		switch {
		case e.Edges.Person != nil:
			ev := exportEvent(e, e.Edges.Person.Name, "")
			ex.Events[eventKey(ev.Person, ev.Group, ev.Type, ev.Date)] = ev
		case e.Edges.Group != nil:
			ev := exportEvent(e, "", e.Edges.Group.Name)
			ex.Events[eventKey(ev.Person, ev.Group, ev.Type, ev.Date)] = ev
		}
	}

	gifts, err := deps.GiftIdeas.ListAll(ctx)
	if err != nil {
		return ex, err
	}
	for _, g := range gifts {
		if g.Edges.Person == nil {
			continue
		}
		gi := GiftIdea{
			Person:     g.Edges.Person.Name,
			Title:      g.Title,
			Notes:      g.Notes,
			PriceCents: g.PriceCents,
			URL:        g.URL,
			Status:     string(g.Status),
		}
		ex.GiftIdeas[giftKey(gi.Person, gi.Title)] = gi
	}

	rels, err := deps.Relationships.ListAll(ctx)
	if err != nil {
		return ex, err
	}
	for _, rel := range rels {
		from, okFrom := names[rel.FromID]
		to, okTo := names[rel.ToID]
		if !okFrom || !okTo {
			continue
		}
		r := Relationship{From: from, To: to, Type: string(rel.Type)}
		if rel.Label != nil {
			r.Label = *rel.Label
		}
		ex.Relationships[relationshipKey(r.From, r.To, r.Type)] = r
	}

	personNotes, err := deps.PersonNotes.ListAll(ctx)
	if err != nil {
		return ex, err
	}
	for _, n := range personNotes {
		if n.Edges.Person == nil {
			continue
		}
		note := PersonNote{Person: n.Edges.Person.Name, Note: n.Note, Date: formatDate(n.NoteDate)}
		ex.PersonNotes[personNoteKey(note.Person, note.Note)] = note
	}

	groupNotes, err := deps.GroupNotes.ListAll(ctx)
	if err != nil {
		return ex, err
	}
	for _, n := range groupNotes {
		if n.Edges.Group == nil {
			continue
		}
		note := GroupNote{Group: n.Edges.Group.Name, Note: n.Note, Date: formatDate(n.NoteDate)}
		ex.GroupNotes[groupNoteKey(note.Group, note.Note)] = note
	}

	rules, err := deps.Rules.ListAll(ctx)
	if err != nil {
		return ex, err
	}
	for _, r := range rules {
		rr := RecurringRule{
			Name:        r.Name,
			PatternType: r.PatternType,
			Nth:         r.Nth,
			Weekday:     r.Weekday,
			Month:       r.Month,
			Day:         r.Day,
			Enabled:     r.Enabled,
		}
		ex.Rules[nameKey(rr.Name)] = rr
	}

	return ex, nil
}

// Preview compares an envelope against the current library. "Skip" means the
// natural key already exists with the same content; "Conflict" means it exists
// with different content (an import will still not overwrite it).
func Preview(existing Existing, env Envelope) PreviewResult {
	var out PreviewResult

	for _, p := range env.People {
		if have, ok := existing.People[nameKey(p.Name)]; ok {
			if samePerson(have, p) {
				out.People.Skip++
			} else {
				out.People.Conflict++
			}
		} else {
			out.People.New++
		}
	}
	for _, g := range env.Groups {
		if have, ok := existing.Groups[nameKey(g.Name)]; ok {
			if have.Description == g.Description {
				out.Groups.Skip++
			} else {
				out.Groups.Conflict++
			}
		} else {
			out.Groups.New++
		}
	}
	for _, t := range env.Tags {
		if existing.Tags[nameKey(t.Name)] {
			out.Tags.Skip++
		} else {
			out.Tags.New++
		}
	}
	for _, e := range env.Events {
		if have, ok := existing.Events[eventKey(e.Person, e.Group, e.Type, e.Date)]; ok {
			if sameEvent(have, e) {
				out.Events.Skip++
			} else {
				out.Events.Conflict++
			}
		} else {
			out.Events.New++
		}
	}
	for _, g := range env.GiftIdeas {
		if have, ok := existing.GiftIdeas[giftKey(g.Person, g.Title)]; ok {
			if sameGift(have, g) {
				out.GiftIdeas.Skip++
			} else {
				out.GiftIdeas.Conflict++
			}
		} else {
			out.GiftIdeas.New++
		}
	}
	for _, r := range env.Relationships {
		if have, ok := existing.Relationships[relationshipKey(r.From, r.To, r.Type)]; ok {
			if have.Label == r.Label {
				out.Relationships.Skip++
			} else {
				out.Relationships.Conflict++
			}
		} else {
			out.Relationships.New++
		}
	}
	for _, n := range env.PersonNotes {
		if have, ok := existing.PersonNotes[personNoteKey(n.Person, n.Note)]; ok {
			if have.Date == n.Date {
				out.PersonNotes.Skip++
			} else {
				out.PersonNotes.Conflict++
			}
		} else {
			out.PersonNotes.New++
		}
	}
	for _, n := range env.GroupNotes {
		if have, ok := existing.GroupNotes[groupNoteKey(n.Group, n.Note)]; ok {
			if have.Date == n.Date {
				out.GroupNotes.Skip++
			} else {
				out.GroupNotes.Conflict++
			}
		} else {
			out.GroupNotes.New++
		}
	}
	for _, r := range env.RecurringRules {
		if have, ok := existing.Rules[nameKey(r.Name)]; ok {
			if sameRule(have, r) {
				out.RecurringRules.Skip++
			} else {
				out.RecurringRules.Conflict++
			}
		} else {
			out.RecurringRules.New++
		}
	}

	return out
}

// Import creates only what is missing. It never updates or deletes existing
// records and never touches users or settings, so running it twice creates
// nothing on the second run.
func Import(ctx context.Context, deps Deps, env Envelope, _ Options) (Result, error) {
	if err := Validate(env); err != nil {
		return Result{}, err
	}
	existing, err := LoadExisting(ctx, deps)
	if err != nil {
		return Result{}, err
	}

	var res Result
	personIDs := existing.PersonIDs
	groupIDs := existing.GroupIDs

	skip := func(entity, name, reason string) {
		res.Skipped = append(res.Skipped, SkipRecord{Entity: entity, Name: name, Reason: reason})
	}

	for _, t := range env.Tags {
		key := nameKey(t.Name)
		if existing.Tags[key] {
			res.Tags.Skipped++
			continue
		}
		if _, err := deps.Tags.FindOrCreate(ctx, t.Name); err != nil {
			res.Tags.Skipped++
			skip("tag", t.Name, err.Error())
			continue
		}
		existing.Tags[key] = true
		res.Tags.Created++
	}

	for _, g := range env.Groups {
		key := nameKey(g.Name)
		if _, ok := groupIDs[key]; ok {
			res.Groups.Skipped++
			continue
		}
		created, err := deps.Groups.Create(ctx, g.Name, g.Description)
		if err != nil {
			res.Groups.Skipped++
			skip("group", g.Name, err.Error())
			continue
		}
		groupIDs[key] = created.ID
		res.Groups.Created++
	}

	for _, p := range env.People {
		key := nameKey(p.Name)
		if _, ok := personIDs[key]; ok {
			res.People.Skipped++
			skip("person", p.Name, "already exists")
			continue
		}
		created, err := deps.People.CreateStructured(ctx, p.Name, p.FirstName, p.MiddleName, p.LastName, p.Notes, "")
		if err != nil {
			res.People.Skipped++
			skip("person", p.Name, err.Error())
			continue
		}
		personIDs[key] = created.ID
		res.People.Created++

		for _, groupName := range p.Groups {
			g, err := deps.Groups.GetOrCreateByName(ctx, groupName)
			if err != nil {
				continue
			}
			groupIDs[nameKey(groupName)] = g.ID
			_ = deps.Groups.AddPerson(ctx, g.ID, created.ID)
		}
		for _, tagName := range p.Tags {
			if err := deps.Tags.AddToPerson(ctx, created.ID, tagName); err != nil {
				continue
			}
		}
	}

	for _, e := range env.Events {
		key := eventKey(e.Person, e.Group, e.Type, e.Date)
		if _, ok := existing.Events[key]; ok {
			res.Events.Skipped++
			continue
		}
		ownerPerson, hasPerson := personIDs[nameKey(e.Person)]
		ownerGroup, hasGroup := groupIDs[nameKey(e.Group)]
		if !hasPerson && !hasGroup {
			res.Events.Skipped++
			skip("event", e.Type+" "+e.Date, "owner not found")
			continue
		}
		date, err := parseDate(e.Date)
		if err != nil {
			res.Events.Skipped++
			skip("event", e.Type+" "+e.Date, "invalid date")
			continue
		}
		system := e.CalendarSystem
		if system == "" {
			system = "gregorian"
		}
		freq := e.Frequency
		if freq == "" {
			freq = recurring.FrequencyAnnual
		}
		var endDate *time.Time
		if e.EndDate != "" {
			if parsed, err := parseDate(e.EndDate); err == nil {
				endDate = &parsed
			}
		}
		schedule := repository.NewEventSchedule(freq, e.Interval, endDate, e.OccurrenceCount)

		var created *ent.Event
		if hasPerson {
			created, err = deps.Events.CreateForPersonWithSchedule(ctx, ownerPerson, e.Type, date, e.Description, system, e.LunarMonth, e.LunarDay, e.LunarLeap, schedule)
		} else {
			created, err = deps.Events.CreateForGroupWithSchedule(ctx, ownerGroup, e.Type, date, e.Description, system, e.LunarMonth, e.LunarDay, e.LunarLeap, schedule)
		}
		if err != nil {
			res.Events.Skipped++
			skip("event", e.Type+" "+e.Date, err.Error())
			continue
		}
		if e.Notes != "" {
			if _, err := deps.Events.SetNotes(ctx, created.ID, e.Notes); err != nil {
				// The event itself exists; a note failure must not create it twice.
				skip("event", e.Type+" "+e.Date, "notes not saved: "+err.Error())
			}
		}
		existing.Events[key] = e
		res.Events.Created++
	}

	for _, g := range env.GiftIdeas {
		key := giftKey(g.Person, g.Title)
		if _, ok := existing.GiftIdeas[key]; ok {
			res.GiftIdeas.Skipped++
			continue
		}
		personID, ok := personIDs[nameKey(g.Person)]
		if !ok {
			res.GiftIdeas.Skipped++
			skip("gift idea", g.Title, "person not found")
			continue
		}
		created, err := deps.GiftIdeas.Create(ctx, personID, g.Title, g.Notes, g.PriceCents, g.URL)
		if err != nil {
			res.GiftIdeas.Skipped++
			skip("gift idea", g.Title, err.Error())
			continue
		}
		if g.Status != "" && g.Status != string(giftidea.StatusIdea) {
			if _, err := deps.GiftIdeas.UpdateStatus(ctx, created.ID, giftidea.Status(g.Status)); err != nil {
				skip("gift idea", g.Title, "status not saved: "+err.Error())
			}
		}
		existing.GiftIdeas[key] = g
		res.GiftIdeas.Created++
	}

	for _, r := range env.Relationships {
		key := relationshipKey(r.From, r.To, r.Type)
		if _, ok := existing.Relationships[key]; ok {
			res.Relationships.Skipped++
			continue
		}
		fromID, okFrom := personIDs[nameKey(r.From)]
		toID, okTo := personIDs[nameKey(r.To)]
		if !okFrom || !okTo {
			res.Relationships.Skipped++
			skip("relationship", r.From+" → "+r.To, "person not found")
			continue
		}
		if _, err := deps.Relationships.Add(ctx, fromID, toID, r.Type, r.Label); err != nil {
			res.Relationships.Skipped++
			skip("relationship", r.From+" → "+r.To, err.Error())
			continue
		}
		existing.Relationships[key] = r
		res.Relationships.Created++
	}

	for _, n := range env.PersonNotes {
		key := personNoteKey(n.Person, n.Note)
		if _, ok := existing.PersonNotes[key]; ok {
			res.PersonNotes.Skipped++
			continue
		}
		personID, ok := personIDs[nameKey(n.Person)]
		if !ok {
			res.PersonNotes.Skipped++
			skip("person note", n.Person, "person not found")
			continue
		}
		date := time.Now().UTC()
		if n.Date != "" {
			parsed, err := parseDate(n.Date)
			if err != nil {
				res.PersonNotes.Skipped++
				skip("person note", n.Person, "invalid date")
				continue
			}
			date = parsed
		}
		if _, err := deps.PersonNotes.Create(ctx, personID, n.Note, date); err != nil {
			res.PersonNotes.Skipped++
			skip("person note", n.Person, err.Error())
			continue
		}
		existing.PersonNotes[key] = n
		res.PersonNotes.Created++
	}

	for _, n := range env.GroupNotes {
		key := groupNoteKey(n.Group, n.Note)
		if _, ok := existing.GroupNotes[key]; ok {
			res.GroupNotes.Skipped++
			continue
		}
		groupID, ok := groupIDs[nameKey(n.Group)]
		if !ok {
			res.GroupNotes.Skipped++
			skip("group note", n.Group, "group not found")
			continue
		}
		date := time.Now().UTC()
		if n.Date != "" {
			parsed, err := parseDate(n.Date)
			if err != nil {
				res.GroupNotes.Skipped++
				skip("group note", n.Group, "invalid date")
				continue
			}
			date = parsed
		}
		if _, err := deps.GroupNotes.Create(ctx, groupID, n.Note, date); err != nil {
			res.GroupNotes.Skipped++
			skip("group note", n.Group, err.Error())
			continue
		}
		existing.GroupNotes[key] = n
		res.GroupNotes.Created++
	}

	for _, r := range env.RecurringRules {
		key := nameKey(r.Name)
		if _, ok := existing.Rules[key]; ok {
			res.RecurringRules.Skipped++
			continue
		}
		created, err := deps.Rules.Create(ctx, r.Name, r.PatternType, r.Nth, r.Weekday, r.Month, r.Day)
		if err != nil {
			res.RecurringRules.Skipped++
			skip("recurring rule", r.Name, err.Error())
			continue
		}
		if !r.Enabled {
			if _, err := deps.Rules.Update(ctx, created.ID, r.Name, r.PatternType, r.Nth, r.Weekday, r.Month, r.Day, false); err != nil {
				skip("recurring rule", r.Name, "enabled flag not saved: "+err.Error())
			}
		}
		existing.Rules[key] = r
		res.RecurringRules.Created++
	}

	return res, nil
}

// exportPerson copies only the natural fields of a person.
func exportPerson(p *ent.Person) Person {
	out := Person{
		Name:       p.Name,
		FirstName:  deref(p.FirstName),
		MiddleName: deref(p.MiddleName),
		LastName:   deref(p.LastName),
		Notes:      p.Notes,
	}
	for _, g := range p.Edges.Groups {
		out.Groups = append(out.Groups, g.Name)
	}
	sort.Strings(out.Groups)
	for _, t := range p.Edges.Tags {
		out.Tags = append(out.Tags, t.Name)
	}
	sort.Strings(out.Tags)
	return out
}

func exportEvent(e *ent.Event, person, group string) Event {
	out := Event{
		Person:          person,
		Group:           group,
		Type:            e.Type,
		Date:            formatDate(e.Date),
		Description:     e.Description,
		Notes:           e.Notes,
		CalendarSystem:  e.CalendarSystem,
		LunarMonth:      e.LunarMonth,
		LunarDay:        e.LunarDay,
		LunarLeap:       e.LunarLeap,
		Frequency:       e.Frequency,
		Interval:        e.Interval,
		OccurrenceCount: e.OccurrenceCount,
	}
	if e.EndDate != nil {
		out.EndDate = formatDate(*e.EndDate)
	}
	return out
}

func samePerson(a, b Person) bool {
	return a.Name == b.Name && a.FirstName == b.FirstName && a.MiddleName == b.MiddleName &&
		a.LastName == b.LastName && a.Notes == b.Notes &&
		strings.Join(a.Groups, "\x00") == strings.Join(b.Groups, "\x00") &&
		strings.Join(a.Tags, "\x00") == strings.Join(b.Tags, "\x00")
}

func sameEvent(a, b Event) bool {
	return a.Person == b.Person && a.Group == b.Group && a.Type == b.Type && a.Date == b.Date &&
		a.Description == b.Description && a.Notes == b.Notes &&
		a.CalendarSystem == b.CalendarSystem && intPtrEqual(a.LunarMonth, b.LunarMonth) &&
		intPtrEqual(a.LunarDay, b.LunarDay) && a.LunarLeap == b.LunarLeap &&
		a.Frequency == b.Frequency && a.Interval == b.Interval &&
		a.EndDate == b.EndDate && intPtrEqual(a.OccurrenceCount, b.OccurrenceCount)
}

func sameGift(a, b GiftIdea) bool {
	return a.Person == b.Person && a.Title == b.Title && a.Notes == b.Notes &&
		intPtrEqual(a.PriceCents, b.PriceCents) && a.URL == b.URL && a.Status == b.Status
}

func sameRule(a, b RecurringRule) bool {
	return a.Name == b.Name && a.PatternType == b.PatternType && a.Nth == b.Nth &&
		a.Weekday == b.Weekday && a.Month == b.Month && a.Day == b.Day && a.Enabled == b.Enabled
}

func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatDate(t time.Time) string {
	return t.UTC().Format(dateLayout)
}

func parseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, s, time.UTC)
}

// nameKey is the case-insensitive natural key for names.
func nameKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func eventKey(person, group, eventType, date string) string {
	owner := "person:" + nameKey(person)
	if strings.TrimSpace(group) != "" {
		owner = "group:" + nameKey(group)
	}
	return owner + "|" + strings.ToLower(strings.TrimSpace(eventType)) + "|" + date
}

func giftKey(person, title string) string {
	return nameKey(person) + "|" + strings.ToLower(strings.TrimSpace(title))
}

func relationshipKey(from, to, relType string) string {
	return nameKey(from) + "|" + nameKey(to) + "|" + strings.ToLower(strings.TrimSpace(relType))
}

func personNoteKey(person, note string) string {
	return nameKey(person) + "|" + note
}

func groupNoteKey(group, note string) string {
	return nameKey(group) + "|" + note
}
