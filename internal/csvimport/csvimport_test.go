package csvimport

import (
	"strings"
	"testing"
	"time"
)

func parseString(t *testing.T, csvText string) *Result {
	t.Helper()
	res, err := Parse(strings.NewReader(csvText))
	if err != nil {
		t.Fatalf("Parse(%q): %v", csvText, err)
	}
	return res
}

func TestParse_HeaderSynonyms(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		row         string
		wantRoles   map[string]int
		wantName    string
		wantDate    string
		wantGroups  []string
		wantTags    []string
		wantNotes   string
		wantFirst   string
		wantLast    string
		wantHeader  bool
		wantPos     bool
		wantRows    int
		wantWarnSub string
	}{
		{
			name:       "canonical header",
			header:     "Name,Birthday,Notes,Groups,Tags",
			row:        "Ada Lovelace,1990-12-10,Math,Family;Friends,smart;female",
			wantRoles:  map[string]int{RoleName: 0, RoleBirthday: 1, RoleNotes: 2, RoleGroups: 3, RoleTags: 4},
			wantName:   "Ada Lovelace",
			wantDate:   "1990-12-10",
			wantGroups: []string{"Family", "Friends"},
			wantTags:   []string{"smart", "female"},
			wantNotes:  "Math",
			wantHeader: true,
		},
		{
			name:       "full name and dob synonyms",
			header:     "Full Name,DOB",
			row:        "Bob Builder,11/02/1985",
			wantRoles:  map[string]int{RoleName: 0, RoleBirthday: 1},
			wantName:   "Bob Builder",
			wantDate:   "1985-02-11",
			wantHeader: true,
		},
		{
			name:       "punctuated bday header",
			header:     "CONTACT,B-DAY",
			row:        "Cleo,1980-01-02",
			wantRoles:  map[string]int{RoleName: 0, RoleBirthday: 1},
			wantName:   "Cleo",
			wantDate:   "1980-01-02",
			wantHeader: true,
		},
		{
			name:       "structured name columns",
			header:     "First Name,Middle,Last Name,Date of Birth",
			row:        "Mary,Jane,Watson,1991/03/04",
			wantRoles:  map[string]int{RoleFirst: 0, RoleMiddle: 1, RoleLast: 2, RoleBirthday: 3},
			wantName:   "Mary Jane Watson",
			wantFirst:  "Mary",
			wantLast:   "Watson",
			wantDate:   "1991-03-04",
			wantHeader: true,
		},
		{
			name:       "category and comment synonyms",
			header:     "name,categories,comment",
			row:        "Dan,Friends;Work,from work",
			wantRoles:  map[string]int{RoleName: 0, RoleGroups: 1, RoleNotes: 2},
			wantName:   "Dan",
			wantGroups: []string{"Friends", "Work"},
			wantNotes:  "from work",
			wantHeader: true,
		},
		{
			name:     "positional fallback",
			header:   "Ada Lovelace,1990-12-10",
			row:      "Bob Builder,11/02/1985",
			wantPos:  true,
			wantRows: 2,
			wantName: "Ada Lovelace",
			wantDate: "1990-12-10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := parseString(t, tt.header+"\n"+tt.row+"\n")
			if res.Mapping.HeaderRow != tt.wantHeader {
				t.Errorf("HeaderRow = %v, want %v (mapping %+v)", res.Mapping.HeaderRow, tt.wantHeader, res.Mapping)
			}
			if res.Mapping.Positional != tt.wantPos {
				t.Errorf("Positional = %v, want %v", res.Mapping.Positional, tt.wantPos)
			}
			for role, idx := range tt.wantRoles {
				if got := res.Mapping.Index(role); got != idx {
					t.Errorf("role %q index = %d, want %d (mapping %+v)", role, got, idx, res.Mapping)
				}
			}
			wantRows := tt.wantRows
			if wantRows == 0 {
				wantRows = 1
			}
			if len(res.Rows) != wantRows {
				t.Fatalf("rows = %d, want %d: %+v", len(res.Rows), wantRows, res.Rows)
			}
			row := res.Rows[0]
			if row.Name != tt.wantName {
				t.Errorf("name = %q, want %q", row.Name, tt.wantName)
			}
			if row.Date != tt.wantDate {
				t.Errorf("date = %q, want %q (raw %q, reason %q)", row.Date, tt.wantDate, row.BirthdayRaw, row.SkipReason)
			}
			if tt.wantFirst != "" && row.First != tt.wantFirst {
				t.Errorf("first = %q, want %q", row.First, tt.wantFirst)
			}
			if tt.wantLast != "" && row.Last != tt.wantLast {
				t.Errorf("last = %q, want %q", row.Last, tt.wantLast)
			}
			if tt.wantNotes != "" && row.Notes != tt.wantNotes {
				t.Errorf("notes = %q, want %q", row.Notes, tt.wantNotes)
			}
			if len(tt.wantGroups) > 0 && strings.Join(row.Groups, "|") != strings.Join(tt.wantGroups, "|") {
				t.Errorf("groups = %v, want %v", row.Groups, tt.wantGroups)
			}
			if len(tt.wantTags) > 0 && strings.Join(row.Tags, "|") != strings.Join(tt.wantTags, "|") {
				t.Errorf("tags = %v, want %v", row.Tags, tt.wantTags)
			}
			if tt.wantPos && len(res.Warnings) == 0 {
				t.Errorf("positional fallback must warn, got none")
			}
		})
	}
}

func TestParse_BOMAndCRLF(t *testing.T) {
	data := "\xef\xbb\xbfName,Birthday\r\nAda,1990-12-10\r\n"
	res := parseString(t, data)
	if len(res.Rows) != 1 {
		t.Fatalf("rows = %d, want 1: %+v", len(res.Rows), res.Rows)
	}
	if res.Rows[0].Name != "Ada" {
		t.Errorf("name = %q, want Ada (BOM not stripped?)", res.Rows[0].Name)
	}
	if res.Rows[0].Date != "1990-12-10" {
		t.Errorf("date = %q, want 1990-12-10", res.Rows[0].Date)
	}
}

func TestParse_QuotedFieldsWithCommasAndNewlines(t *testing.T) {
	data := "Name,Birthday,Notes\n" +
		"\"Doe, Jane\",1990-12-10,\"line one\nline two\"\n"
	res := parseString(t, data)
	if len(res.Rows) != 1 {
		t.Fatalf("rows = %d, want 1: %+v", len(res.Rows), res.Rows)
	}
	row := res.Rows[0]
	if row.Name != "Doe, Jane" {
		t.Errorf("name = %q, want %q", row.Name, "Doe, Jane")
	}
	if row.Notes != "line one\nline two" {
		t.Errorf("notes = %q, want embedded newline preserved", row.Notes)
	}
	if row.Date != "1990-12-10" {
		t.Errorf("date = %q, want 1990-12-10", row.Date)
	}
}

func TestParse_RaggedRows(t *testing.T) {
	data := "Name,Birthday,Notes\n" +
		"Ada,1990-12-10\n" +
		"Bob,1991-01-02,has note,extra,columns\n"
	res := parseString(t, data)
	if len(res.Rows) != 2 {
		t.Fatalf("rows = %d, want 2: %+v", len(res.Rows), res.Rows)
	}
	if res.Rows[0].Name != "Ada" || res.Rows[0].Notes != "" {
		t.Errorf("short row parsed as %+v", res.Rows[0])
	}
	if res.Rows[1].Notes != "has note" {
		t.Errorf("long row notes = %q, want %q", res.Rows[1].Notes, "has note")
	}
	for _, r := range res.Rows {
		if r.SkipReason != "" {
			t.Errorf("row %d unexpectedly skipped: %q", r.Row, r.SkipReason)
		}
	}
}

func TestParse_DateFormats(t *testing.T) {
	tests := []struct {
		in       string
		year     int
		month    time.Month
		day      int
		yearless bool
	}{
		{"1990-06-08", 1990, time.June, 8, false},
		{"08/06/1990", 1990, time.June, 8, false},
		{"08.06.1990", 1990, time.June, 8, false},
		{"1990/06/08", 1990, time.June, 8, false},
		{"2 Jan 1990", 1990, time.January, 2, false},
		{"Jan 2, 1990", 1990, time.January, 2, false},
		{"8 June 1990", 1990, time.June, 8, false},
		{"January 1990", 1990, time.January, 1, false},
		{"--0608", 0, time.June, 8, true},
		{"--06-08", 0, time.June, 8, true},
		{"-0608", 0, time.June, 8, true},
		{"0608", 0, time.June, 8, true},
		{"08-06", 0, time.June, 8, true},
		{"6/8", 0, time.August, 6, true},
		{"June 8", 0, time.June, 8, true},
		{"8 June", 0, time.June, 8, true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, yearless, ok := ParseDate(tt.in)
			if !ok {
				t.Fatalf("ParseDate(%q) failed", tt.in)
			}
			if got.Year() != tt.year || got.Month() != tt.month || got.Day() != tt.day {
				t.Errorf("ParseDate(%q) = %v, want %d-%02d-%02d", tt.in, got, tt.year, int(tt.month), tt.day)
			}
			if yearless != tt.yearless {
				t.Errorf("ParseDate(%q) yearless = %v, want %v", tt.in, yearless, tt.yearless)
			}
		})
	}
}

func TestParse_DateFormatsInFile(t *testing.T) {
	data := "Name,Birthday\n" +
		"A,1990-06-08\n" +
		"B,08/06/1990\n" +
		"C,08.06.1990\n" +
		"D,1990/06/08\n" +
		"E,2 Jan 1990\n" +
		"F,\"Jan 2, 1990\"\n"

	res := parseString(t, data)
	if len(res.Rows) != 6 {
		t.Fatalf("rows = %d, want 6: %+v", len(res.Rows), res.Rows)
	}
	for _, row := range res.Rows {
		if row.SkipReason != "" {
			t.Fatalf("row %d (%q) unexpectedly skipped: %q", row.Row, row.Name, row.SkipReason)
		}
		if row.Date != "1990-06-08" && !(row.Name == "E" || row.Name == "F") {
			t.Errorf("row %d (%q) date = %q, want 1990-06-08", row.Row, row.Name, row.Date)
		}
		if (row.Name == "E" || row.Name == "F") && row.Date != "1990-01-02" {
			t.Errorf("row %d (%q) date = %q, want 1990-01-02", row.Row, row.Name, row.Date)
		}
	}
}

func TestParse_YearlessBirthdays(t *testing.T) {
	data := "Name,Birthday\n" +
		"A,--0608\n" +
		"B,0608\n" +
		"C,June 8\n"
	res := parseString(t, data)
	for _, row := range res.Rows {
		if row.SkipReason != "" {
			t.Fatalf("row %q skipped: %q", row.Name, row.SkipReason)
		}
		if !row.Yearless {
			t.Errorf("row %q not flagged yearless", row.Name)
		}
		if row.Date != "--06-08" {
			t.Errorf("row %q date = %q, want --06-08", row.Name, row.Date)
		}
	}
}

func TestParse_AmbiguousDayMonthWarning(t *testing.T) {
	// Only ambiguous values: both 03 and 04 are valid months, so the file
	// cannot prove a convention and must warn while reading DD/MM.
	res := parseString(t, "Name,Birthday\nA,03/04/1990\n")
	if len(res.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(res.Rows))
	}
	if res.Rows[0].Date != "1990-04-03" {
		t.Errorf("ambiguous date = %q, want 1990-04-03 (DD/MM)", res.Rows[0].Date)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected an ambiguity warning, got none")
	}
	if !strings.Contains(res.Warnings[0], "ambiguous") {
		t.Errorf("warning = %q, want mention of ambiguity", res.Warnings[0])
	}
}

func TestParse_UnambiguousDayFirstNoWarning(t *testing.T) {
	res := parseString(t, "Name,Birthday\nA,25/04/1990\n")
	if res.Rows[0].Date != "1990-04-25" {
		t.Errorf("date = %q, want 1990-04-25", res.Rows[0].Date)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "ambiguous") {
			t.Errorf("unexpected ambiguity warning: %q", w)
		}
	}
}

func TestParse_MonthFirstEvidence(t *testing.T) {
	// 04/25 proves month-first, so 03/04 in the same file is March 4.
	res := parseString(t, "Name,Birthday\nA,04/25/1990\nB,03/04/1990\n")
	if res.Rows[0].Date != "1990-04-25" {
		t.Errorf("row A = %q, want 1990-04-25", res.Rows[0].Date)
	}
	if res.Rows[1].Date != "1990-03-04" {
		t.Errorf("row B = %q, want 1990-03-04 (month-first)", res.Rows[1].Date)
	}
}

func TestParse_MixedConventionsWarn(t *testing.T) {
	res := parseString(t, "Name,Birthday\nA,25/04/1990\nB,04/25/1990\nC,03/04/1990\n")
	if len(res.Warnings) == 0 || !strings.Contains(strings.Join(res.Warnings, " "), "mixes") {
		t.Fatalf("expected a mixed-convention warning, got %v", res.Warnings)
	}
	if res.Rows[2].Date != "1990-04-03" {
		t.Errorf("ambiguous date in mixed file = %q, want DD/MM 1990-04-03", res.Rows[2].Date)
	}
}

func TestParse_UnparseableDateReason(t *testing.T) {
	res := parseString(t, "Name,Birthday\nAda,not-a-date\n")
	if len(res.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(res.Rows))
	}
	want := `unparseable date "not-a-date"`
	if res.Rows[0].SkipReason != want {
		t.Errorf("reason = %q, want %q", res.Rows[0].SkipReason, want)
	}
	importable, skipped := res.Counts()
	if importable != 0 || skipped != 1 {
		t.Errorf("counts = (%d,%d), want (0,1)", importable, skipped)
	}
}

func TestParse_MissingNameAndBirthday(t *testing.T) {
	res := parseString(t, "Name,Birthday\n,1990-01-01\nBob,\n")
	if len(res.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(res.Rows))
	}
	if res.Rows[0].SkipReason != ReasonMissingName {
		t.Errorf("row 0 reason = %q, want %q", res.Rows[0].SkipReason, ReasonMissingName)
	}
	if res.Rows[1].SkipReason != ReasonMissingBirthday {
		t.Errorf("row 1 reason = %q, want %q", res.Rows[1].SkipReason, ReasonMissingBirthday)
	}
}

func TestParse_DuplicateWithinFile(t *testing.T) {
	res := parseString(t, "Name,Birthday\nAda,1990-01-01\nada ,1991-02-02\nBob,1990-03-03\n")
	if len(res.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(res.Rows))
	}
	if res.Rows[1].SkipReason != ReasonDuplicateWithinFile {
		t.Errorf("row 1 reason = %q, want %q", res.Rows[1].SkipReason, ReasonDuplicateWithinFile)
	}
	importable, skipped := res.Counts()
	if importable != 2 || skipped != 1 {
		t.Errorf("counts = (%d,%d), want (2,1)", importable, skipped)
	}
}

func TestMarkExistingAndUnknownGroups(t *testing.T) {
	res := parseString(t, "Name,Birthday,Groups\nAda,1990-01-01,Friends\nBob,1990-02-02,Family;New Group\n")
	res.MarkExisting(map[string]bool{NameKey("ADA"): true})

	if res.Rows[0].SkipReason != ReasonDuplicateExisting {
		t.Errorf("row 0 reason = %q, want %q", res.Rows[0].SkipReason, ReasonDuplicateExisting)
	}
	unknown := res.UnknownGroups(map[string]bool{NameKey("friends"): true})
	if len(unknown) != 2 || unknown[0] != "Family" || unknown[1] != "New Group" {
		// Only Bob is importable and he carries both groups.
		t.Errorf("unknown groups = %v, want [Family New Group]", unknown)
	}
}

func TestParse_SemicolonDelimitedFile(t *testing.T) {
	res := parseString(t, "Name;Birthday;Groups\nAda;1990-01-01;Family,Friends\n")
	if len(res.Rows) != 1 {
		t.Fatalf("rows = %d, want 1: %+v", len(res.Rows), res.Rows)
	}
	if res.Rows[0].Name != "Ada" || res.Rows[0].Date != "1990-01-01" {
		t.Errorf("row = %+v", res.Rows[0])
	}
	if len(res.Rows[0].Groups) != 2 {
		t.Errorf("groups = %v, want 2 entries", res.Rows[0].Groups)
	}
}

func TestParse_EmptyFile(t *testing.T) {
	res := parseString(t, "")
	if len(res.Rows) != 0 {
		t.Errorf("rows = %d, want 0", len(res.Rows))
	}
}

func TestHeaderRole_Synonyms(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"name", RoleName},
		{"Full Name", RoleName},
		{"fullname", RoleName},
		{"contact", RoleName},
		{"First Name", RoleFirst},
		{"firstname", RoleFirst},
		{"Given Name", RoleFirst},
		{"middle", RoleMiddle},
		{"Last Name", RoleLast},
		{"lastname", RoleLast},
		{"Surname", RoleLast},
		{"Family Name", RoleLast},
		{"birthday", RoleBirthday},
		{"birthdate", RoleBirthday},
		{"Birth Date", RoleBirthday},
		{"Date of Birth", RoleBirthday},
		{"DOB", RoleBirthday},
		{"bday", RoleBirthday},
		{"notes", RoleNotes},
		{"note", RoleNotes},
		{"Comment", RoleNotes},
		{"comments", RoleNotes},
		{"groups", RoleGroups},
		{"group", RoleGroups},
		{"Category", RoleGroups},
		{"categories", RoleGroups},
		{"tags", RoleTags},
		{"Tag", RoleTags},
		{"unrelated column", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := headerRole(tt.header); got != tt.want {
			t.Errorf("headerRole(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}
