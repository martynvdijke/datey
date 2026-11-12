// Package giftgap builds the optional reminder line that nudges the user to
// add a gift idea for a person whose birthday or anniversary is coming up.
//
// It is deliberately free of storage and transport concerns: callers pass the
// person's open-gift-idea count (status "idea") and the configured APP_URL, so
// the same logic serves the web UI, the daily reminder message and any future
// channel without duplicating phrasing or link building.
package giftgap

import "strconv"

const (
	// Fragment is the anchor of the gift-ideas section on the person detail
	// page. Deep links point at it so the user lands on the form.
	Fragment = "#gift-ideas"

	// baseLine is the sentence shown when the person has no open gift idea.
	baseLine = "No gift idea yet — add one"
)

// Line returns the extra reminder line for a person with no open gift idea.
//
// It returns "" when the line does not apply: the person already has at least
// one open ("idea") gift idea, or no usable person ID was supplied. When
// appURL is empty the line is returned without a link, so notification
// channels never receive a dangling "add one: " suffix.
func Line(personID, openIdeas int, appURL string) string {
	if personID <= 0 || openIdeas > 0 {
		return ""
	}
	link := PersonURL(personID, appURL)
	if link == "" {
		return baseLine
	}
	return baseLine + ": " + link
}

// PersonURL builds the absolute deep link to a person's gift ideas section, or
// "" when appURL is blank (the caller has no externally reachable base URL).
func PersonURL(personID int, appURL string) string {
	if personID <= 0 || appURL == "" {
		return ""
	}
	base := appURL
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + "/people/" + strconv.Itoa(personID) + Fragment
}
