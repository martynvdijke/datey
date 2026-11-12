// Package tzutil centralizes every timezone decision Datey makes.
//
// Event dates are stored as calendar dates; whether "3 days left" is true
// depends on which calendar the viewer is standing on. This package therefore
// owns the three primitives everyone needs: resolving a zone name to a
// *time.Location, taking the calendar date "today" in that zone, and counting
// calendar days between two dates (DST safe, because both sides are normalized
// to a fixed UTC midnight before subtracting).
//
// Resolution order for an event is: person timezone, then the instance-wide
// default (TIMEZONE, applied by the orchestrator via SetDefault), then
// time.Local. An empty name always means "inherit", so installs that configure
// nothing keep the pre-timezone behaviour.
package tzutil

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrUnknownZone is returned by Validate for a name that is neither empty,
// "Local", "UTC", nor a zone the IANA database knows.
var ErrUnknownZone = errors.New("unknown IANA timezone")

// defaultName holds the instance-wide default zone name. It starts empty
// (= time.Local) and is replaced by the orchestrator after config load and on
// settings hot-reload.
var defaultName string

// SetDefault records the instance-wide default zone name. An unknown name is
// ignored so a bad value can never take the process down at runtime; startup
// validation reports it instead.
func SetDefault(name string) {
	name = strings.TrimSpace(name)
	if name != "" && !Valid(name) {
		return
	}
	defaultName = name
}

// DefaultName returns the configured instance-wide default zone name, or ""
// when none is set.
func DefaultName() string { return defaultName }

// Default returns the instance-wide default location (never nil).
func Default() *time.Location { return Location(defaultName) }

// Location resolves an IANA zone name to a *time.Location. It never returns
// nil: an empty name, "Local", or an unknown name all fall back to time.Local
// so callers never need a nil check.
func Location(name string) *time.Location {
	name = strings.TrimSpace(name)
	switch name {
	case "", "Local":
		return time.Local
	}
	if loc, err := time.LoadLocation(name); err == nil && loc != nil {
		return loc
	}
	return time.Local
}

// Valid reports whether name is an acceptable zone: empty (inherit), "Local",
// "UTC", or a name in the IANA database.
func Valid(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == "Local" {
		return true
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// Validate returns a user-facing error for an unusable zone name, or nil.
// The message names the rejected value so a form can show it verbatim.
func Validate(name string) error {
	name = strings.TrimSpace(name)
	if Valid(name) {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrUnknownZone, name)
}

// Resolve returns the effective zone for a person: their own timezone when set
// and valid, otherwise the instance default, otherwise time.Local. Never nil.
func Resolve(personTimezone string) *time.Location {
	if tz := strings.TrimSpace(personTimezone); tz != "" && Valid(tz) {
		return Location(tz)
	}
	return Default()
}

// ResolveWith is Resolve with an explicit global default, for callers that do
// not use the package default (tests, or code holding a config value).
func ResolveWith(personTimezone, globalTimezone string) *time.Location {
	if tz := strings.TrimSpace(personTimezone); tz != "" && Valid(tz) {
		return Location(tz)
	}
	return Location(globalTimezone)
}

// Today returns midnight of now's calendar date in loc, expressed in UTC.
//
// The result carries the zone's calendar Y/M/D but a UTC clock, so date
// arithmetic across a DST transition stays a whole number of days. Callers must
// only compare it with other values built the same way (Today, or a stored
// event date that is already midnight UTC).
func Today(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	local := now.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// DaysBetween returns the number of calendar days from one date to another.
// Only the year/month/day components matter; both are normalized to UTC
// midnight first so a DST-shortened or -lengthened day still counts as one day.
func DaysBetween(from, to time.Time) int {
	f := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	t := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(t.Sub(f).Hours() / 24)
}

// DaysUntil returns the calendar days from "today in loc" to eventDate. A
// negative result means the date is in the past. eventDate is read by its own
// calendar components, which is correct for stored date-only values held at
// midnight UTC.
func DaysUntil(eventDate time.Time, loc *time.Location, now time.Time) int {
	return DaysBetween(Today(now, loc), eventDate)
}

// FormatDate renders t in loc using layout.
func FormatDate(t time.Time, loc *time.Location, layout string) string {
	if loc == nil {
		loc = time.Local
	}
	return t.In(loc).Format(layout)
}

// ZoneGroup is one region of the curated zone picker.
type ZoneGroup struct {
	Region string
	Zones  []string
}

// commonZones is the curated IANA list offered in the person form's select.
// It is intentionally a fixed list (not the full tz database, which is ~600
// entries) with a free-text fallback for anything missing.
var commonZones = []ZoneGroup{
	{"UTC", []string{"UTC"}},
	{"Europe", []string{
		"Europe/London", "Europe/Dublin", "Europe/Lisbon", "Europe/Madrid",
		"Europe/Paris", "Europe/Brussels", "Europe/Amsterdam", "Europe/Berlin",
		"Europe/Zurich", "Europe/Vienna", "Europe/Prague", "Europe/Warsaw",
		"Europe/Rome", "Europe/Malta", "Europe/Athens", "Europe/Helsinki",
		"Europe/Bucharest", "Europe/Sofia", "Europe/Kyiv", "Europe/Istanbul",
		"Europe/Moscow", "Europe/Oslo", "Europe/Stockholm", "Europe/Copenhagen",
		"Atlantic/Reykjavik", "Europe/Belgrade", "Europe/Zagreb", "Europe/Budapest",
	}},
	{"Americas", []string{
		"America/St_Johns", "America/Halifax", "America/New_York",
		"America/Toronto", "America/Chicago", "America/Mexico_City",
		"America/Denver", "America/Phoenix", "America/Los_Angeles",
		"America/Vancouver", "America/Anchorage", "Pacific/Honolulu",
		"America/Bogota", "America/Lima", "America/Santiago", "America/Sao_Paulo",
		"America/Argentina/Buenos_Aires", "America/Caracas",
	}},
	{"Africa", []string{
		"Africa/Casablanca", "Africa/Lagos", "Africa/Accra", "Africa/Cairo",
		"Africa/Johannesburg", "Africa/Nairobi", "Africa/Addis_Ababa",
	}},
	{"Asia", []string{
		"Asia/Jerusalem", "Asia/Beirut", "Asia/Riyadh", "Asia/Dubai",
		"Asia/Tehran", "Asia/Karachi", "Asia/Kolkata", "Asia/Kathmandu",
		"Asia/Dhaka", "Asia/Bangkok", "Asia/Jakarta", "Asia/Singapore",
		"Asia/Hong_Kong", "Asia/Shanghai", "Asia/Taipei", "Asia/Seoul",
		"Asia/Tokyo", "Asia/Manila",
	}},
	{"Oceania", []string{
		"Australia/Perth", "Australia/Adelaide", "Australia/Brisbane",
		"Australia/Sydney", "Australia/Melbourne", "Pacific/Auckland",
		"Pacific/Fiji",
	}},
}

// CommonZoneGroups returns the curated picker groups, sorted with the UTC
// pseudo-group first and the rest alphabetically.
func CommonZoneGroups() []ZoneGroup {
	out := make([]ZoneGroup, 0, len(commonZones))
	for _, g := range commonZones {
		cp := ZoneGroup{Region: g.Region, Zones: append([]string(nil), g.Zones...)}
		out = append(out, cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Region == "UTC" {
			return true
		}
		if out[j].Region == "UTC" {
			return false
		}
		return out[i].Region < out[j].Region
	})
	return out
}
