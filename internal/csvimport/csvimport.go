// Package csvimport parses spreadsheet exports (CSV) of people and birthdays
// into previewable, importable rows.
//
// The primary input is a birthdays list exported from Excel or Google Sheets,
// so the parser is deliberately forgiving: it sniffs the delimiter, strips a
// UTF-8 BOM, accepts ragged rows, and recognises a set of common column
// synonyms. When no header row can be recognised it falls back to the
// positional layout name,birthday and says so in the result warnings.
//
// Birthdays may be year-less. Those reuse the vCard semantics already used by
// the rest of the app (internal/vcard.ParseBDAY) and are represented as year 0
// so the age logic treats them as date-only entries ("no age").
package csvimport

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/internal/person"
	"github.com/datey/datey/internal/vcard"
)

// Column roles recognised in a header row.
const (
	RoleName     = "name"
	RoleFirst    = "first"
	RoleMiddle   = "middle"
	RoleLast     = "last"
	RoleBirthday = "birthday"
	RoleNotes    = "notes"
	RoleGroups   = "groups"
	RoleTags     = "tags"
)

// Skip reasons attached to rows that cannot be imported. They are rendered
// verbatim in the preview, so they stay short and human-readable.
const (
	ReasonMissingName         = "missing name"
	ReasonMissingBirthday     = "missing birthday"
	ReasonDuplicateExisting   = "duplicate of existing person"
	ReasonDuplicateWithinFile = "duplicate within file"
)

// FieldMap records where a role was found, for display in the preview.
type FieldMap struct {
	Role   string
	Header string
	Index  int
}

// Mapping is the detected column layout of the file.
type Mapping struct {
	Fields     []FieldMap
	HeaderRow  bool
	Positional bool
}

// Index returns the column index for role, or -1 when the role is absent.
func (m Mapping) Index(role string) int {
	for _, f := range m.Fields {
		if f.Role == role {
			return f.Index
		}
	}
	return -1
}

// Row is one parsed person. Date is formatted as YYYY-MM-DD, or as --MM-DD for
// a year-less birthday; an empty Date means no usable date was found.
type Row struct {
	Row         int      `json:"row"`
	Name        string   `json:"name"`
	First       string   `json:"first,omitempty"`
	Middle      string   `json:"middle,omitempty"`
	Last        string   `json:"last,omitempty"`
	Notes       string   `json:"notes,omitempty"`
	Date        string   `json:"date,omitempty"`
	BirthdayRaw string   `json:"birthday_raw,omitempty"`
	Yearless    bool     `json:"yearless,omitempty"`
	Groups      []string `json:"groups,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	SkipReason  string   `json:"skip_reason,omitempty"`
}

// Result is the outcome of parsing a file.
type Result struct {
	Rows     []Row
	Mapping  Mapping
	Warnings []string
}

// Total reports the number of data rows considered.
func (r *Result) Total() int { return len(r.Rows) }

// Counts reports how many rows are importable and how many are skipped.
func (r *Result) Counts() (importable, skipped int) {
	for _, row := range r.Rows {
		if row.SkipReason == "" {
			importable++
		} else {
			skipped++
		}
	}
	return importable, skipped
}

// MarkExisting flags importable rows whose name already exists. existing is
// keyed by NameKey.
func (r *Result) MarkExisting(existing map[string]bool) {
	for i := range r.Rows {
		if r.Rows[i].SkipReason != "" {
			continue
		}
		if existing[NameKey(r.Rows[i].Name)] {
			r.Rows[i].SkipReason = ReasonDuplicateExisting
		}
	}
}

// UnknownGroups returns the distinct group names used by importable rows that
// are not in known (keyed by NameKey). Groups are only created on confirm, so
// the preview can state exactly which ones are new.
func (r *Result) UnknownGroups(known map[string]bool) []string {
	var out []string
	seen := make(map[string]bool)
	for _, row := range r.Rows {
		if row.SkipReason != "" {
			continue
		}
		for _, g := range row.Groups {
			key := NameKey(g)
			if key == "" || known[key] || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, g)
		}
	}
	return out
}

// NameKey normalises a person or group name for duplicate comparison.
func NameKey(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), " ")
}

// FormatDate renders a parsed birthday in payload form: --MM-DD for year-less
// dates, YYYY-MM-DD otherwise.
func FormatDate(t time.Time) string {
	if t.Year() <= 0 {
		return vcard.FormatBDAY(t)
	}
	return t.Format("2006-01-02")
}

// Parse reads a CSV file and returns its rows, detected mapping and file-level
// warnings. A malformed CSV stream (unterminated quote, bad encoding) is
// returned as an error; individual bad rows are not.
func Parse(r io.Reader) (*Result, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))

	reader := csv.NewReader(bytes.NewReader(raw))
	reader.FieldsPerRecord = -1 // ragged rows are tolerated
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	reader.Comma = sniffDelimiter(raw)

	var (
		records [][]string
		lines   []int
	)
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv: %w", err)
		}
		line, _ := reader.FieldPos(0)
		records = append(records, rec)
		lines = append(lines, line)
	}

	result := &Result{}
	if len(records) == 0 {
		return result, nil
	}

	// First non-empty record decides whether there is a header row.
	first := -1
	for i, rec := range records {
		if !allEmpty(rec) {
			first = i
			break
		}
	}
	if first < 0 {
		return result, nil
	}

	start := first
	if mapping, ok := detectHeader(records[first]); ok {
		result.Mapping = mapping
		start = first + 1
	} else {
		result.Mapping = Mapping{
			Positional: true,
			Fields: []FieldMap{
				{Role: RoleName, Header: "column 1", Index: 0},
				{Role: RoleBirthday, Header: "column 2", Index: 1},
			},
		}
		result.Warnings = append(result.Warnings,
			"No header row recognised; assuming column 1 is the name and column 2 is the birthday.")
	}

	nameIdx := result.Mapping.Index(RoleName)
	firstIdx := result.Mapping.Index(RoleFirst)
	middleIdx := result.Mapping.Index(RoleMiddle)
	lastIdx := result.Mapping.Index(RoleLast)
	bdayIdx := result.Mapping.Index(RoleBirthday)
	notesIdx := result.Mapping.Index(RoleNotes)
	groupsIdx := result.Mapping.Index(RoleGroups)
	tagsIdx := result.Mapping.Index(RoleTags)

	// Infer the day-first / month-first convention from the whole file before
	// parsing rows, so an ambiguous 03/04/1990 is read consistently.
	conv, ambiguous, mixed := inferConvention(records[start:], bdayIdx)
	switch {
	case mixed:
		result.Warnings = append(result.Warnings,
			"The file mixes day-first and month-first dates; ambiguous dates are read as DD/MM.")
	case ambiguous && conv == convUnknown:
		result.Warnings = append(result.Warnings,
			`Some dates are ambiguous (e.g. "03/04/1990"); they are read as DD/MM. Use YYYY-MM-DD to be explicit.`)
	}

	var monthYearWarned bool
	seenNames := make(map[string]bool)

	for i := start; i < len(records); i++ {
		rec := records[i]
		if allEmpty(rec) {
			continue
		}

		row := Row{Row: lines[i]}
		row.Name = strings.TrimSpace(cell(rec, nameIdx))
		row.First = strings.TrimSpace(cell(rec, firstIdx))
		row.Middle = strings.TrimSpace(cell(rec, middleIdx))
		row.Last = strings.TrimSpace(cell(rec, lastIdx))
		row.Notes = strings.TrimSpace(cell(rec, notesIdx))
		row.Groups = splitList(cell(rec, groupsIdx))
		row.Tags = splitList(cell(rec, tagsIdx))
		if row.Name == "" {
			row.Name = person.DisplayName(row.First, row.Middle, row.Last, "")
		}

		row.BirthdayRaw = strings.TrimSpace(cell(rec, bdayIdx))
		switch {
		case row.Name == "":
			row.SkipReason = ReasonMissingName
		case row.BirthdayRaw == "":
			row.SkipReason = ReasonMissingBirthday
		default:
			t, yearless, monthYearOnly, ok := parseDate(row.BirthdayRaw, conv)
			if !ok {
				row.SkipReason = fmt.Sprintf("unparseable date %q", row.BirthdayRaw)
				break
			}
			row.Date = FormatDate(t)
			row.Yearless = yearless
			if monthYearOnly && !monthYearWarned {
				monthYearWarned = true
				result.Warnings = append(result.Warnings,
					fmt.Sprintf(`Date %q has no day; day 1 is assumed.`, row.BirthdayRaw))
			}
		}

		if row.SkipReason == "" {
			key := NameKey(row.Name)
			if seenNames[key] {
				row.SkipReason = ReasonDuplicateWithinFile
			} else {
				seenNames[key] = true
			}
		}

		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

// detectHeader maps a candidate header row to a column layout. It only accepts
// the row when at least one identity column (name/first/last) or the birthday
// column is present, so an ordinary data row is never mistaken for a header.
func detectHeader(rec []string) (Mapping, bool) {
	mapping := Mapping{HeaderRow: true}
	seen := make(map[string]bool)
	for i, cellValue := range rec {
		role := headerRole(cellValue)
		if role == "" || seen[role] {
			continue
		}
		seen[role] = true
		mapping.Fields = append(mapping.Fields, FieldMap{
			Role:   role,
			Header: strings.TrimSpace(cellValue),
			Index:  i,
		})
	}
	if len(mapping.Fields) == 0 {
		return Mapping{}, false
	}
	if !seen[RoleName] && !seen[RoleFirst] && !seen[RoleLast] && !seen[RoleBirthday] {
		return Mapping{}, false
	}
	return mapping, true
}

// headerRole maps a header cell to a column role, matching synonyms
// case-insensitively and ignoring punctuation and spacing ("Full_Name",
// "full name" and "FULLNAME" all match).
func headerRole(cellValue string) string {
	switch normalizeHeader(cellValue) {
	case "name", "fullname", "contact":
		return RoleName
	case "firstname", "givenname":
		return RoleFirst
	case "middle", "middlename":
		return RoleMiddle
	case "lastname", "surname", "familyname":
		return RoleLast
	case "birthday", "birthdate", "dateofbirth", "dob", "bday":
		return RoleBirthday
	case "notes", "note", "comment", "comments":
		return RoleNotes
	case "groups", "group", "category", "categories":
		return RoleGroups
	case "tags", "tag":
		return RoleTags
	}
	return ""
}

// normalizeHeader lowercases a header cell and removes everything that is not
// a letter or digit, so punctuation and spacing never break a synonym match.
func normalizeHeader(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitList splits a groups/tags cell on commas or semicolons, trimming
// whitespace and dropping empties and duplicates.
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' })
	var out []string
	seen := make(map[string]bool)
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" || seen[strings.ToLower(f)] {
			continue
		}
		seen[strings.ToLower(f)] = true
		out = append(out, f)
	}
	return out
}

// convention is the numeric day/month order inferred for slash-separated dates.
type convention int

const (
	convUnknown convention = iota
	convDMY
	convMDY
	convMixed
)

// inferConvention scans every slash-separated full date in the birthday column
// and decides whether the file is day-first or month-first. A value like
// 25/04/1990 proves day-first; 04/25/1990 proves month-first. Files that prove
// both are reported as mixed (ambiguous values then fall back to DD/MM).
func inferConvention(records [][]string, bdayIdx int) (conv convention, ambiguous, mixed bool) {
	if bdayIdx < 0 {
		return convUnknown, false, false
	}
	var dmy, mdy bool
	for _, rec := range records {
		parts, sep := splitNumeric(strings.TrimSpace(cell(rec, bdayIdx)))
		if sep != '/' || len(parts) != 3 {
			continue
		}
		if len(parts[0]) > 2 || len(parts[1]) > 2 || len(parts[2]) != 4 {
			continue
		}
		a, errA := strconv.Atoi(parts[0])
		b, errB := strconv.Atoi(parts[1])
		if errA != nil || errB != nil {
			continue
		}
		if a <= 12 && b <= 12 {
			ambiguous = true
		}
		if a > 12 {
			dmy = true
		}
		if b > 12 {
			mdy = true
		}
	}
	switch {
	case dmy && mdy:
		return convMixed, ambiguous, true
	case mdy:
		return convMDY, ambiguous, false
	case dmy:
		return convDMY, ambiguous, false
	default:
		return convUnknown, ambiguous, false
	}
}

// parseDate parses one birthday cell. yearless reports a month/day-only value;
// monthYearOnly reports a value that only carried a month and year (day 1 is
// assumed so the caller can warn).
func parseDate(raw string, conv convention) (t time.Time, yearless, monthYearOnly, ok bool) {
	s := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if s == "" {
		return time.Time{}, false, false, false
	}

	// Reuse the vCard BDAY parser first: it owns --MM-DD, -MMDD, MMDD and ISO
	// semantics, so CSV and vCard imports agree on year-less birthdays.
	if parsed, ok := vcard.ParseBDAY(s); ok {
		if parsed.Year() == 0 {
			return parsed, true, false, true
		}
		return parsed, false, false, true
	}

	if parts, sep := splitNumeric(s); len(parts) > 0 {
		switch {
		case len(parts) == 3 && len(parts[0]) == 4:
			// YYYY-MM-DD / YYYY/MM/DD / YYYY.MM.DD
			return buildDate(parts[0], parts[1], parts[2])
		case len(parts) == 3:
			year, month, day := parts[2], parts[1], parts[0]
			if sep == '/' && conv == convMDY {
				month, day = parts[0], parts[1]
			}
			return buildDate(year, month, day)
		case len(parts) == 2:
			// Year-less day/month. Read day-first, matching DD/MM/YYYY.
			return buildYearless(parts[0], parts[1])
		}
	}

	// Month-name formats, full dates before year-less ones.
	fullLayouts := []string{
		"2 Jan 2006", "2 January 2006",
		"Jan 2, 2006", "January 2, 2006",
		"Jan 2 2006", "January 2 2006",
	}
	for _, layout := range fullLayouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed, false, false, true
		}
	}

	// Month + year only: accept it but flag the invented day.
	for _, layout := range []string{"January 2006", "Jan 2006"} {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed, false, true, true
		}
	}

	yearlessLayouts := []string{"2 January", "2 Jan", "January 2", "Jan 2"}
	for _, layout := range yearlessLayouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed, true, false, true
		}
	}

	return time.Time{}, false, false, false
}

// ParseDate parses a birthday cell using the default day-first convention.
func ParseDate(raw string) (time.Time, bool, bool) {
	t, yearless, _, ok := parseDate(raw, convDMY)
	return t, yearless, ok
}

// splitNumeric splits a numeric date on a single consistent separator. It
// returns nil when the string is not purely numeric/separators.
func splitNumeric(s string) ([]string, byte) {
	var sep byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '/' || c == '.' || c == '-' {
			if sep == 0 {
				sep = c
			} else if c != sep {
				return nil, 0
			}
		}
	}
	if sep == 0 {
		return nil, 0
	}
	parts := strings.Split(s, string(sep))
	if len(parts) < 2 || len(parts) > 3 {
		return nil, 0
	}
	for _, p := range parts {
		if p == "" {
			return nil, 0
		}
		for i := 0; i < len(p); i++ {
			if p[i] < '0' || p[i] > '9' {
				return nil, 0
			}
		}
	}
	return parts, sep
}

func buildDate(ys, ms, ds string) (time.Time, bool, bool, bool) {
	y, err1 := strconv.Atoi(ys)
	m, err2 := strconv.Atoi(ms)
	d, err3 := strconv.Atoi(ds)
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, false, false, false
	}
	if y < 1000 || y > 9999 || m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false, false, false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != y || int(t.Month()) != m || t.Day() != d {
		return time.Time{}, false, false, false
	}
	return t, false, false, true
}

func buildYearless(ds, ms string) (time.Time, bool, bool, bool) {
	d, err1 := strconv.Atoi(ds)
	m, err2 := strconv.Atoi(ms)
	if err1 != nil || err2 != nil {
		return time.Time{}, false, false, false
	}
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false, false, false
	}
	t := time.Date(0, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != 0 || int(t.Month()) != m || t.Day() != d {
		return time.Time{}, false, false, false
	}
	return t, true, false, true
}

// sniffDelimiter picks ';' for files that clearly use it (common in European
// Excel exports) and ',' otherwise. It only looks at the first line so a
// comma-separated file with semicolons inside a cell is not misdetected.
func sniffDelimiter(raw []byte) rune {
	line := raw
	if i := bytes.IndexByte(raw, '\n'); i >= 0 {
		line = raw[:i]
	}
	commas := bytes.Count(line, []byte(","))
	semicolons := bytes.Count(line, []byte(";"))
	if commas == 0 && semicolons > 0 {
		return ';'
	}
	return ','
}

// cell returns rec[i], or "" when the row is ragged.
func cell(rec []string, i int) string {
	if i < 0 || i >= len(rec) {
		return ""
	}
	return rec[i]
}

func allEmpty(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
