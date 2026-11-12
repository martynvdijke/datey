// Package search queries the FTS5 full-text index maintained by internal/db.
//
// The index is derived data owned by SQLite triggers; this package only reads
// it. User input is never handed to FTS5 verbatim — it is turned into a MATCH
// expression built from quoted, sanitised terms so FTS5 operators and syntax
// cannot leak through (see BuildMatchExpr).
package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"html/template"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Entity identifies the kind of document a result came from. The values match
// the `entity` column written by the index triggers in internal/db.
type Entity string

const (
	EntityPerson     Entity = "person"
	EntityPersonNote Entity = "person_note"
	EntityGiftIdea   Entity = "gift_idea"
	EntityEvent      Entity = "event"
	EntityGroup      Entity = "group"
	EntityGroupNote  Entity = "group_note"
)

const (
	// MaxTerms caps how many words of a query are searched. Beyond this the
	// extra terms add almost no value but make each MATCH more expensive.
	MaxTerms = 8
	// MaxQueryRunes caps the whole query; the rest is ignored.
	MaxQueryRunes = 200
	// maxTermRunes caps one token so a pasted blob cannot make the expression
	// pathological.
	maxTermRunes = 64
	// DefaultLimit caps the number of results returned per search.
	DefaultLimit = 50

	snippetOpen  = "\x01"
	snippetClose = "\x02"
)

// ErrIndexUnavailable reports that the FTS5 index is missing or unusable, for
// example because the SQLite build has no FTS5 module. The web layer renders a
// friendly "search unavailable" state for it instead of failing the request.
var ErrIndexUnavailable = errors.New("search index unavailable")

// entityOrder is the order groups are returned in: the things users look for
// first (people) come first.
var entityOrder = []Entity{
	EntityPerson,
	EntityPersonNote,
	EntityGiftIdea,
	EntityEvent,
	EntityGroup,
	EntityGroupNote,
}

// Result is one matching document.
type Result struct {
	Entity Entity
	ID     int
	// ParentEntity/ParentID identify the owning person or group, so the UI can
	// link a note back to its person and show the owner's name.
	ParentEntity Entity
	ParentID     int
	// Title is the document's own title (person/group name, gift title, event
	// type). It is empty for notes.
	Title string
	// ParentTitle is the name of the owning person or group, when there is one.
	ParentTitle string
	// Snippet is HTML with the matched terms wrapped in <mark>. All text is
	// escaped here, so templates may render it unescaped.
	Snippet template.HTML
	// URL is the in-app location of the result.
	URL string
}

// Group collects the results for one entity type, in rank order.
type Group struct {
	Entity  Entity
	Results []Result
}

// Count returns the number of results in the group.
func (g Group) Count() int { return len(g.Results) }

// Searcher reads the full-text index.
type Searcher struct {
	db *sql.DB
	// Limit caps the number of results. Zero means DefaultLimit.
	Limit int
}

// New returns a Searcher over db.
func New(db *sql.DB) *Searcher {
	return &Searcher{db: db, Limit: DefaultLimit}
}

// searchQuery ranks matches with bm25 (title weighted 8x over body) and asks
// SQLite for a highlighted snippet from whichever column matched best. The
// correlated sub-selects attach the owner's display name without a second
// round trip; they only run for the rows that survive the LIMIT.
const searchQuery = `
SELECT
	s.entity,
	s.entity_id,
	s.parent_entity,
	s.parent_id,
	s.title,
	snippet(search_index, -1, char(1), char(2), '…', 12),
	CASE
		WHEN s.entity IN ('person_note', 'gift_idea')
			THEN (SELECT name FROM persons WHERE persons.id = s.parent_id)
		WHEN s.entity = 'group_note'
			THEN (SELECT name FROM groups WHERE groups.id = s.parent_id)
		WHEN s.entity = 'event' AND s.parent_entity = 'person'
			THEN (SELECT name FROM persons WHERE persons.id = s.parent_id)
		WHEN s.entity = 'event' AND s.parent_entity = 'group'
			THEN (SELECT name FROM groups WHERE groups.id = s.parent_id)
		ELSE ''
	END
FROM search_index AS s
WHERE search_index MATCH ?
ORDER BY bm25(search_index, 0.0, 0.0, 0.0, 0.0, 8.0, 1.0)
LIMIT ?
`

// Search returns matches grouped by entity. An empty or whitespace-only query
// returns no results and no error.
func (s *Searcher) Search(ctx context.Context, query string) ([]Group, error) {
	expr := BuildMatchExpr(query)
	if expr == "" {
		return nil, nil
	}
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("%w: no database", ErrIndexUnavailable)
	}
	limit := s.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}

	rows, err := s.db.QueryContext(ctx, searchQuery, expr, limit)
	if err != nil {
		if isMissingIndex(err) {
			return nil, fmt.Errorf("%w: %v", ErrIndexUnavailable, err)
		}
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byEntity := make(map[Entity][]Result, len(entityOrder))
	for rows.Next() {
		var (
			entity       string
			entityID     int
			parentEntity sql.NullString
			parentID     sql.NullInt64
			title        sql.NullString
			snippet      sql.NullString
			parentTitle  sql.NullString
		)
		if err := rows.Scan(&entity, &entityID, &parentEntity, &parentID, &title, &snippet, &parentTitle); err != nil {
			return nil, fmt.Errorf("search scan: %w", err)
		}
		ent := Entity(strings.TrimSpace(entity))
		if ent == "" {
			continue
		}
		res := Result{
			Entity:      ent,
			ID:          entityID,
			Title:       strings.TrimSpace(title.String),
			ParentTitle: strings.TrimSpace(parentTitle.String),
			Snippet:     renderSnippet(snippet.String),
		}
		if parentEntity.Valid {
			res.ParentEntity = Entity(strings.TrimSpace(parentEntity.String))
		}
		if parentID.Valid {
			res.ParentID = int(parentID.Int64)
		}
		res.URL = urlFor(res)
		byEntity[ent] = append(byEntity[ent], res)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search rows: %w", err)
	}

	groups := make([]Group, 0, len(byEntity))
	for _, ent := range entityOrder {
		if results := byEntity[ent]; len(results) > 0 {
			groups = append(groups, Group{Entity: ent, Results: results})
		}
	}
	return groups, nil
}

// urlFor maps a result onto the page that shows it. Notes and gift ideas have
// no page of their own: they live on the owner's page, which also keeps the
// web layer from hardcoding any route knowledge.
func urlFor(r Result) string {
	switch r.Entity {
	case EntityPerson:
		return fmt.Sprintf("/people/%d", r.ID)
	case EntityGroup:
		return fmt.Sprintf("/groups/%d", r.ID)
	case EntityPersonNote, EntityGiftIdea:
		if r.ParentID > 0 {
			return fmt.Sprintf("/people/%d", r.ParentID)
		}
	case EntityGroupNote:
		if r.ParentID > 0 {
			return fmt.Sprintf("/groups/%d", r.ParentID)
		}
	case EntityEvent:
		switch r.ParentEntity {
		case EntityPerson:
			if r.ParentID > 0 {
				return fmt.Sprintf("/people/%d", r.ParentID)
			}
		case EntityGroup:
			if r.ParentID > 0 {
				return fmt.Sprintf("/groups/%d", r.ParentID)
			}
		}
		return "/calendar"
	}
	return "/people"
}

// BuildMatchExpr turns free text into a safe FTS5 MATCH expression.
//
// Every term is quoted, so FTS5 operators (AND, OR, NOT, NEAR), prefix stars,
// column filters, parentheses, quotes and the caret are treated as literal
// text instead of syntax. The final term gets a trailing `*` so a partially
// typed word ("birt") still matches ("birthday"). Unsupported punctuation and
// symbols are separators, which keeps unicode letters (including combining
// marks) intact.
func BuildMatchExpr(query string) string {
	terms := sanitizeTerms(query)
	if len(terms) == 0 {
		return ""
	}
	if len(terms) > MaxTerms {
		terms = terms[:MaxTerms]
	}
	var b strings.Builder
	for i, term := range terms {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte('"')
		b.WriteString(term)
		b.WriteByte('"')
		if i == len(terms)-1 {
			b.WriteByte('*')
		}
	}
	return b.String()
}

// sanitizeTerms splits on anything that is not a letter, digit or combining
// mark, caps the input length and each term, and drops empty terms.
func sanitizeTerms(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if utf8.RuneCountInString(query) > MaxQueryRunes {
		query = string([]rune(query)[:MaxQueryRunes])
	}

	terms := make([]string, 0, MaxTerms)
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		term := current.String()
		current.Reset()
		if utf8.RuneCountInString(term) > maxTermRunes {
			term = string([]rune(term)[:maxTermRunes])
		}
		terms = append(terms, term)
	}

	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return terms
}

// renderSnippet escapes the snippet text and then turns the sentinel markers
// inserted by SQLite's snippet() into <mark> tags. Escaping first means text
// from the database can never inject markup.
func renderSnippet(raw string) template.HTML {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	escaped := html.EscapeString(raw)
	escaped = strings.ReplaceAll(escaped, snippetOpen, "<mark>")
	escaped = strings.ReplaceAll(escaped, snippetClose, "</mark>")
	return template.HTML(escaped) //nolint:gosec // text was escaped above; only <mark> is added
}

// isMissingIndex reports whether err is "the index does not exist", which is
// recoverable, as opposed to a genuine query failure.
func isMissingIndex(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "no such module") ||
		strings.Contains(msg, "unable to use function match")
}
