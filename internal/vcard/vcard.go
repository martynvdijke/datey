package vcard

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	govcard "github.com/emersion/go-vcard"
	"github.com/datey/datey/ent"
)

// ParsedContact holds the fields extracted from a single vCard entry.
type ParsedContact struct {
	Name  string
	Notes string
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
// FN → name, NOTE → notes. All other properties are appended to notes.
func ToContact(card govcard.Card) ParsedContact {
	pc := ParsedContact{
		Name: card.Value(govcard.FieldFormattedName),
	}

	var noteParts []string
	if note := card.Value(govcard.FieldNote); note != "" {
		noteParts = append(noteParts, note)
	}

	for k, fields := range card {
		if k == govcard.FieldFormattedName || k == govcard.FieldNote || k == govcard.FieldVersion {
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

// Encode serialises one or more contacts to vCard 3.0 format.
func Encode(contacts []*ent.Contact) ([]byte, error) {
	var buf bytes.Buffer

	for _, c := range contacts {
		card := ToCard(c.Name, c.Notes)
		enc := govcard.NewEncoder(&buf)
		if err := enc.Encode(card); err != nil {
			return nil, fmt.Errorf("encode vCard for %q: %w", c.Name, err)
		}
	}

	return buf.Bytes(), nil
}

// EncodeSingle serialises a single contact to vCard 3.0 format.
func EncodeSingle(c *ent.Contact) ([]byte, error) {
	return Encode([]*ent.Contact{c})
}

// SanitizeFilename converts a contact name to a safe filename.
func SanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	return name
}
