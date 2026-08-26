package vcard

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	govcard "github.com/emersion/go-vcard"
)

// ParsedContact holds the fields extracted from a single vCard entry.
type ParsedContact struct {
	Name                string
	Notes               string
	Birthday            *time.Time
	BirthdayParseFailed bool
	Gender              string
	FamilyName          string
	GivenName           string
	MiddleName          string
	UID                 string
	Rev                 string
	RawData             string
	Categories          []string
}

// Parse reads a .vcf file and returns all parsed contacts.
// Malformed entries are silently skipped. Returns nil, nil for an empty file.
func Parse(r io.Reader) ([]ParsedContact, error) {
	rawBytes, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	rawText := string(rawBytes)
	rawBlocks := extractRawBlocks(rawText)

	dec := govcard.NewDecoder(strings.NewReader(rawText))
	var contacts []ParsedContact
	blockIdx := 0

	for {
		card, err := dec.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			blockIdx++
			continue
		}

		var rawData string
		if blockIdx < len(rawBlocks) {
			rawData = rawBlocks[blockIdx]
		}
		blockIdx++

		pc := ToContact(card, rawData)
		if pc.Name == "" {
			continue
		}
		contacts = append(contacts, pc)
	}

	if len(contacts) == 0 {
		return nil, nil
	}

	return contacts, nil
}

// extractRawBlocks slices the raw text into individual vCard blocks
// by finding BEGIN:VCARD / END:VCARD boundaries.
func extractRawBlocks(rawText string) []string {
	var blocks []string
	rest := rawText
	for {
		start := strings.Index(rest, "BEGIN:VCARD")
		if start < 0 {
			break
		}
		end := strings.Index(rest[start:], "END:VCARD")
		if end < 0 {
			break
		}
		end += start + len("END:VCARD")
		blocks = append(blocks, rest[start:end])
		rest = rest[end:]
	}
	return blocks
}

// ToContact maps a vCard card to a ParsedContact.
// FN → name, BDAY → Birthday, GENDER → Gender, N → FamilyName/GivenName,
// NOTE/TEL/EMAIL/ADR → Notes in human-readable format.
// Unknown properties are silently dropped.
func ToContact(card govcard.Card, rawData string) ParsedContact {
	pc := ParsedContact{
		Name:    card.Value(govcard.FieldFormattedName),
		RawData: rawData,
	}

	// Parse BDAY: full dates (YYYY-MM-DD, YYYYMMDD, RFC3339) and, when no year
	// is present, year-less dates (--MM-DD, --MMDD, -MMDD, MMDD). Year-less
	// values parse to year 0 so the age logic treats them as date-only entries.
	if bday := card.Value(govcard.FieldBirthday); bday != "" {
		if t, ok := ParseBDAY(bday); ok {
			pc.Birthday = &t
		} else {
			pc.BirthdayParseFailed = true
		}
	}

	// Extract GENDER and map to human-readable form.
	if gender := card.Value(govcard.FieldGender); gender != "" {
		if idx := strings.IndexByte(gender, ';'); idx >= 0 {
			pc.Gender = gender[:idx]
		} else {
			pc.Gender = gender
		}
		pc.Gender = genderLabel(pc.Gender)
	}

	// Extract structured name (N). When FN is missing, reconstruct the full
	// display name — including any middle (additional) name — from N.
	if name := card.Name(); name != nil {
		pc.FamilyName = name.FamilyName
		pc.GivenName = name.GivenName
		pc.MiddleName = name.AdditionalName
		if pc.Name == "" {
			pc.Name = strings.Join(nameFields(name), " ")
		}
	}

	// Extract provider bookkeeping for sync purposes. These are never shown
	// as contact notes; they are persisted as sync state instead.
	pc.UID = card.Value(govcard.FieldUID)
	pc.Rev = card.Value(govcard.FieldRevision)

	// Extract CATEGORIES as group membership. Prefer parsing from rawData
	// to correctly handle escaped commas (\,) per vCard spec; fall back to
	// card.Categories() if raw is unavailable.
	if rawData != "" {
		pc.Categories = categoriesFromRaw(rawData)
	} else if cats := card.Categories(); len(cats) == 1 && cats[0] == "" {
		pc.Categories = nil
	} else {
		cats := card.Categories()
		// Filter empty entries (library returns [""] for missing)
		var out []string
		for _, c := range cats {
			if t := strings.TrimSpace(c); t != "" {
				out = append(out, t)
			}
		}
		pc.Categories = out
	}

	// Build human-readable notes from structured contact fields. Provider
	// bookkeeping (UID, REV, SOURCE, etc.) remains available in RawData but is
	// never shown as contact notes.
	var noteParts []string
	if note := card.Value(govcard.FieldNote); note != "" {
		noteParts = append(noteParts, "Note: "+note)
	}
	if pc.Gender != "" {
		noteParts = append(noteParts, "Gender: "+pc.Gender)
	}
	if tel := card.Value(govcard.FieldTelephone); tel != "" {
		noteParts = append(noteParts, "Phone: "+tel)
	}
	if email := card.Value(govcard.FieldEmail); email != "" {
		noteParts = append(noteParts, "Email: "+email)
	}
	if adr := card.Address(); adr != nil {
		addrParts := buildAddressParts(adr)
		if len(addrParts) > 0 {
			noteParts = append(noteParts, "Address: "+strings.Join(addrParts, ", "))
		}
	}

	if len(noteParts) > 0 {
		pc.Notes = strings.Join(noteParts, "\n")
	}

	return pc
}

// nameFields returns the non-empty components of a structured name in display
// order: given, additional (middle), family, honorific prefixes/suffixes.
func nameFields(n *govcard.Name) []string {
	fields := []string{n.GivenName, n.AdditionalName, n.FamilyName}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// ParseBDAY parses a vCard BDAY value into a time.Time. Full dates are tried
// first (YYYY-MM-DD, YYYYMMDD, RFC3339). Year-less dates (--MM-DD, --MMDD,
// -MMDD, MMDD) parse to year 0, which the age logic treats as a date-only
// entry with no usable birth year. Impossible values (e.g. --1399) are
// rejected and ok is false.
func ParseBDAY(s string) (time.Time, bool) {
	// Order matters: full-date layouts must be tried before year-less ones.
	layouts := []string{
		"2006-01-02",
		"20060102",
		time.RFC3339,
		"20060102T150405Z",
		"--01-02",
		"--0102",
		"-0102",
		"0102",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// genderLabel maps a vCard Sex code to a human-readable label.
func genderLabel(s string) string {
	switch s {
	case "M":
		return "Male"
	case "F":
		return "Female"
	case "O":
		return "Other"
	case "N":
		return "None"
	case "U":
		return "Unknown"
	}
	return s
}

// buildAddressParts assembles the non-empty components of an address field
// into a slice suitable for joining with ", ".
func buildAddressParts(a *govcard.Address) []string {
	var parts []string
	if a.StreetAddress != "" {
		parts = append(parts, a.StreetAddress)
	}
	if a.ExtendedAddress != "" {
		parts = append(parts, a.ExtendedAddress)
	}
	if a.Locality != "" {
		parts = append(parts, a.Locality)
	}
	if a.Region != "" {
		parts = append(parts, a.Region)
	}
	if a.PostalCode != "" {
		parts = append(parts, a.PostalCode)
	}
	if a.Country != "" {
		parts = append(parts, a.Country)
	}
	return parts
}

// ToCard creates a vCard Card from name and notes.
func ToCard(name, notes string) govcard.Card {
	card := make(govcard.Card)
	card.SetValue(govcard.FieldVersion, "3.0")
	card.SetValue(govcard.FieldFormattedName, name)
	card.SetValue(govcard.FieldProductID, "-//Datey//EN")
	if notes != "" {
		card.SetValue(govcard.FieldNote, notes)
	}
	return card
}

// NameNotes holds a name and notes pair for vCard encoding.
type NameNotes struct {
	Name       string
	Notes      string
	Categories []string
}

// encodeCategoriesValue joins categories with proper vCard TEXT escaping:
// backslash, newline, and comma are escaped; comma is the list separator.
func encodeCategoriesValue(cats []string) string {
	escaped := make([]string, 0, len(cats))
	for _, c := range cats {
		c = strings.ReplaceAll(c, "\\", "\\\\")
		c = strings.ReplaceAll(c, "\n", "\\n")
		c = strings.ReplaceAll(c, ",", "\\,")
		escaped = append(escaped, c)
	}
	return strings.Join(escaped, ",")
}

// categoriesFromRaw extracts CATEGORIES from raw vCard text, handling
// escaped commas correctly (split on unescaped commas, unescape per part).
func categoriesFromRaw(raw string) []string {
	// Find CATEGORIES line (case-insensitive, may have params).
	var line string
	for _, l := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		// Handle folded lines already unfolded by extractRawBlocks? Keep simple.
		upper := strings.ToUpper(strings.TrimSpace(l))
		// Extract key before : or ;
		sep := strings.Index(l, ":")
		semi := strings.Index(l, ";")
		keyEnd := sep
		if semi >= 0 && semi < sep {
			keyEnd = semi
		}
		if keyEnd < 0 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(l[:keyEnd]))
		if key == "CATEGORIES" {
			colon := strings.Index(l, ":")
			if colon >= 0 {
				line = l[colon+1:]
				break
			}
		}
		_ = upper
	}
	if line == "" {
		return nil
	}
	// Split on commas not preceded by backslash.
	var out []string
	var cur strings.Builder
	escaped := false
	for _, r := range line {
		if escaped {
			// Preserve escaped char for later unescaping; just add r
			// Backslash escape is handled by writing r directly after previous
			// backslash was already written.
			cur.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			cur.WriteRune(r)
			continue
		}
		if r == ',' {
			part := cur.String()
			// Unescape: \ -> \, \n -> newline, \, -> comma
			part = strings.ReplaceAll(part, "\\\\", "\\")
			part = strings.ReplaceAll(part, "\\n", "\n")
			part = strings.ReplaceAll(part, "\\,", ",")
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		part := cur.String()
		part = strings.ReplaceAll(part, "\\\\", "\\")
		part = strings.ReplaceAll(part, "\\n", "\n")
		part = strings.ReplaceAll(part, "\\,", ",")
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// injectCategories replaces or adds a CATEGORIES line in an encoded vCard
// buffer with a correctly escaped categories value, avoiding double-escaping
// from the generic formatValue.
func injectCategories(encoded []byte, cats []string) []byte {
	if len(cats) == 0 {
		return encoded
	}
	val := encodeCategoriesValue(cats)
	line := "CATEGORIES:" + val + "\r\n"
	s := string(encoded)
	// Replace existing CATEGORIES line if present, otherwise insert before END:VCARD
	if idx := strings.Index(s, "CATEGORIES:"); idx >= 0 {
		end := strings.Index(s[idx:], "\r\n")
		if end >= 0 {
			s = s[:idx] + line + s[idx+end+2:]
			return []byte(s)
		}
	}
	// Insert before END:VCARD
	if idx := strings.Index(s, "END:VCARD"); idx >= 0 {
		s = s[:idx] + line + s[idx:]
	}
	return []byte(s)
}

// Encode serialises one or more name/notes pairs to vCard 3.0 format.
func Encode(items []NameNotes) ([]byte, error) {
	var buf bytes.Buffer

	for _, it := range items {
		card := ToCard(it.Name, it.Notes)
		var tmp bytes.Buffer
		enc := govcard.NewEncoder(&tmp)
		if err := enc.Encode(card); err != nil {
			return nil, fmt.Errorf("encode vCard for %q: %w", it.Name, err)
		}
		data := tmp.Bytes()
		if len(it.Categories) > 0 {
			data = injectCategories(data, it.Categories)
		}
		buf.Write(data)
	}

	return buf.Bytes(), nil
}

// EncodeSingle serialises a single name/notes pair to vCard 3.0 format.
func EncodeSingle(name, notes string) ([]byte, error) {
	return Encode([]NameNotes{{Name: name, Notes: notes}})
}

// EncodeSingleWithCategories serialises a single person with categories.
func EncodeSingleWithCategories(name, notes string, categories []string) ([]byte, error) {
	return Encode([]NameNotes{{Name: name, Notes: notes, Categories: categories}})
}

// SyncContact describes a contact to be written to a remote CardDAV address
// book during a sync push. When structured name parts are present they are
// emitted as the vCard N property alongside the FN display name.
type SyncContact struct {
	Name       string
	Notes      string
	Birthday   *time.Time
	UID        string
	Rev        string
	FirstName  string
	MiddleName string
	LastName   string
	Categories []string
}

// EncodeContact serialises a single contact to vCard 3.0 format including the
// BDAY (full or year-less) and, when present, the provider UID and REV. The
// REV is only written when it already exists on the remote so the server keeps
// authoritative revision values; a zero value omits it.
func EncodeContact(c SyncContact) ([]byte, error) {
	card := make(govcard.Card)
	card.SetValue(govcard.FieldVersion, "3.0")
	card.SetValue(govcard.FieldFormattedName, c.Name)
	card.SetValue(govcard.FieldProductID, "-//Datey//EN")
	// Structured N (family;given;additional;;) so remote address books keep
	// the first/middle/last split; only emitted when any part is known.
	if c.FirstName != "" || c.MiddleName != "" || c.LastName != "" {
		card.SetName(&govcard.Name{
			FamilyName:     c.LastName,
			GivenName:      c.FirstName,
			AdditionalName: c.MiddleName,
		})
	}
	if c.UID != "" {
		card.SetValue(govcard.FieldUID, c.UID)
	}
	if c.Rev != "" {
		card.SetValue(govcard.FieldRevision, c.Rev)
	}
	if c.Birthday != nil {
		card.SetValue(govcard.FieldBirthday, FormatBDAY(*c.Birthday))
	}
	if c.Notes != "" {
		card.SetValue(govcard.FieldNote, c.Notes)
	}

	var buf bytes.Buffer
	enc := govcard.NewEncoder(&buf)
	if err := enc.Encode(card); err != nil {
		return nil, fmt.Errorf("encode vCard for %q: %w", c.Name, err)
	}
	data := buf.Bytes()
	if len(c.Categories) > 0 {
		data = injectCategories(data, c.Categories)
	}
	return data, nil
}

// FormatBDAY renders a parsed birthday back to vCard BDAY syntax. Year-less
// dates (year 0) are emitted as --MM-DD so the month/day round-trips without
// inventing a year.
func FormatBDAY(t time.Time) string {
	if t.Year() == 0 {
		return fmt.Sprintf("--%02d-%02d", int(t.Month()), t.Day())
	}
	return t.Format("2006-01-02")
}

// SanitizeFilename converts a contact name to a safe filename.
func SanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	return name
}
