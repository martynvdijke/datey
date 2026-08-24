// Package person holds person-name derivation helpers shared by the web
// handlers, repositories, and vCard/CardDAV sync code.
package person

import "strings"

// DisplayName joins the non-empty structured name parts into the display
// name, mirroring vCard display order (given, additional, family). When every
// part is empty it falls back so legacy rows that only have a display name
// keep working.
func DisplayName(first, middle, last, fallback string) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{first, middle, last} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return fallback
	}
	return strings.Join(parts, " ")
}

// SplitDisplayName heuristically splits a legacy display name into structured
// parts for edit-form prefill: the first token becomes the first name and the
// remainder becomes the last name. The middle part stays empty — assigning it
// automatically would guess wrong more often than not, and the user can move
// tokens between fields before saving.
func SplitDisplayName(name string) (first, middle, last string) {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "", "", ""
	}
	first = fields[0]
	if len(fields) > 1 {
		last = strings.Join(fields[1:], " ")
	}
	return first, middle, last
}
