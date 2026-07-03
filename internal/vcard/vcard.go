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
	Name        string
	Notes       string
	Birthday    *time.Time
	Gender      string
	FamilyName  string
	GivenName   string
}

// Parse reads a .vcf file and returns all parsed contacts.
// Malformed entries are silently skipped. Returns nil, nil for an empty file.
func Parse(r io.Reader) ([]ParsedContact, error) {
	dec := govcard.NewDecoder(r)
	var contacts []ParsedContact

	for {
		card, err := dec.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		pc := ToContact(card)
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

// ToContact maps a vCard card to a ParsedContact.
// FN → name, BDAY → Birthday, GENDER → Gender, N → FamilyName/GivenName,
// NOTE/TEL/EMAIL/ADR → Notes in human-readable format.
// Unknown properties are silently dropped.
func ToContact(card govcard.Card) ParsedContact {
	pc := ParsedContact{
		Name: card.Value(govcard.FieldFormattedName),
	}

	// Parse BDAY: supports YYYYMMDD (v4.0 basic) and YYYY-MM-DD (v3.0 extended).
	if bday := card.Value(govcard.FieldBirthday); bday != "" {
		for _, layout := range []string{"20060102", "2006-01-02"} {
			if t, err := time.Parse(layout, bday); err == nil {
				pc.Birthday = &t
				break
			}
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

	// Extract structured name (N).
	if name := card.Name(); name != nil {
		pc.FamilyName = name.FamilyName
		pc.GivenName = name.GivenName
	}

	// Build human-readable notes from NOTE, TEL, EMAIL, ADR, GENDER,
	// plus any unknown properties as "KEY: value" fallback.
	var noteParts []string
	if note := card.Value(govcard.FieldNote); note != "" {
		noteParts = append(noteParts, note)
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

	// Preserve any remaining (unknown) properties as "KEY: value" fallback.
	// This ensures fields like UID, SOURCE, PRODID, REV from real-world
	// vCards aren't lost.
	structured := map[string]bool{
		govcard.FieldFormattedName: true,
		govcard.FieldVersion:       true,
		govcard.FieldBirthday:      true,
		govcard.FieldGender:        true,
		govcard.FieldName:          true,
		govcard.FieldNote:          true,
		govcard.FieldTelephone:     true,
		govcard.FieldEmail:         true,
		govcard.FieldAddress:       true,
	}
	for k, fields := range card {
		if structured[k] {
			continue
		}
		for _, f := range fields {
			if f.Value != "" {
				noteParts = append(noteParts, k+": "+f.Value)
			}
		}
	}

	if len(noteParts) > 0 {
		pc.Notes = strings.Join(noteParts, "\n")
	}

	return pc
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
	Name  string
	Notes string
}

// Encode serialises one or more name/notes pairs to vCard 3.0 format.
func Encode(items []NameNotes) ([]byte, error) {
	var buf bytes.Buffer

	for _, it := range items {
		card := ToCard(it.Name, it.Notes)
		enc := govcard.NewEncoder(&buf)
		if err := enc.Encode(card); err != nil {
			return nil, fmt.Errorf("encode vCard for %q: %w", it.Name, err)
		}
	}

	return buf.Bytes(), nil
}

// EncodeSingle serialises a single name/notes pair to vCard 3.0 format.
func EncodeSingle(name, notes string) ([]byte, error) {
	return Encode([]NameNotes{{Name: name, Notes: notes}})
}

// SanitizeFilename converts a contact name to a safe filename.
func SanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	return name
}
