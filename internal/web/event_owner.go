package web

import (
	"strconv"

	"github.com/datey/datey/ent"
)

// eventOwnerName resolves the display name for an event's owner: the linked
// person, else the contact, else the owning group (suffixed to make the
// attribution explicit in feeds and lists).
func eventOwnerName(e *ent.Event) string {
	if e == nil {
		return ""
	}
	if p := e.Edges.Person; p != nil {
		return p.Name
	}
	if c := e.Edges.Contact; c != nil {
		return c.Name
	}
	if g := e.Edges.Group; g != nil {
		return g.Name + " (group)"
	}
	return ""
}

// eventOwnerURL returns the detail-page path for the event's owner, or "".
func eventOwnerURL(e *ent.Event) string {
	if e == nil {
		return ""
	}
	if p := e.Edges.Person; p != nil {
		return "/people/" + strconv.Itoa(p.ID)
	}
	if g := e.Edges.Group; g != nil {
		return "/groups/" + strconv.Itoa(g.ID)
	}
	return ""
}
