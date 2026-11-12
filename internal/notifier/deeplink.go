package notifier

import (
	"strconv"
	"strings"
)

// PersonURL builds the absolute URL of a person's detail page from the
// configured APP_URL. It returns "" when no externally reachable base URL is
// configured, so callers can skip the link instead of emitting a dangling one.
func PersonURL(appURL string, personID int) string {
	if appURL == "" || personID <= 0 {
		return ""
	}
	return strings.TrimRight(appURL, "/") + "/people/" + strconv.Itoa(personID)
}

// PersonLinkLine returns the extra message line pointing at the person, or ""
// when no link can be built. Callers append it to a reminder body.
func PersonLinkLine(appURL string, personID int) string {
	link := PersonURL(appURL, personID)
	if link == "" {
		return ""
	}
	return "View: " + link
}
