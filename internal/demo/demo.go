// Package demo provisions the shared demo account used by the hosted Datey
// instance and seeds a small, realistic address book on a fresh database.
//
// The account is deliberately public: its handle and password are advertised
// on the login page so a visitor can try the app without registering. It is a
// normal, read-write, non-admin user.
package demo

import (
	"context"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/giftidea"
	"github.com/datey/datey/ent/group"
	"github.com/datey/datey/ent/relationship"
	"github.com/datey/datey/ent/tag"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
)

var handleRE = regexp.MustCompile(`^[a-z0-9_]{3,24}$`)

const (
	demoLocale = "en"

	alexNotes   = "Partner. Loves hiking, pour-over coffee and quiet Sundays."
	samNotes    = "Old friend from university. Always up for a long phone call."
	jordanNotes = "Family. Keeps the group chat alive and remembers every birthday."
)

// Ensure creates or repairs the demo account and, only on an empty address
// book, seeds a small demo dataset. It is safe to call on every boot.
func Ensure(ctx context.Context, client *ent.Client, cfg *config.Config, users *repository.UserRepository) error {
	if cfg == nil || !cfg.DemoUserEnabled {
		return nil
	}
	if client == nil || users == nil {
		return fmt.Errorf("demo: client and user repository are required")
	}

	handle := strings.ToLower(strings.TrimSpace(cfg.DemoUserHandle))
	if !handleRE.MatchString(handle) {
		return fmt.Errorf("DEMO_USER_HANDLE must be 3-24 characters of a-z, 0-9 or _")
	}
	if cfg.DemoUserPassword == "" {
		return fmt.Errorf("DEMO_USER_PASSWORD must not be empty")
	}
	email := strings.ToLower(strings.TrimSpace(cfg.DemoUserEmail))
	if email != "" && !validEmail(email) {
		return fmt.Errorf("DEMO_USER_EMAIL must be a valid email address")
	}

	now := time.Now().UTC()

	u, err := users.GetByUsername(ctx, handle)
	if ent.IsNotFound(err) {
		u, err = createDemoUser(ctx, client, handle, email, cfg.DemoUserPassword, now)
	}
	if err != nil {
		return fmt.Errorf("demo user: %w", err)
	}

	// Never take over an account that happens to use the demo handle but
	// belongs to someone else.
	if email != "" && u.Email != nil && *u.Email != "" && !strings.EqualFold(*u.Email, email) {
		return fmt.Errorf("DEMO_USER_HANDLE %q already belongs to another account", handle)
	}
	// A demo account must never be an admin. Refuse rather than demote (and
	// never reset the password of) an existing admin account.
	if u.Role == user.RoleAdmin {
		return fmt.Errorf("DEMO_USER_HANDLE %q belongs to an admin account; refusing to take it over", handle)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.DemoUserPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	update := client.User.UpdateOneID(u.ID).
		SetPasswordHash(string(hash)).
		SetRole(user.RoleUser).
		SetUpdatedAt(now)
	if u.Locale == nil {
		update.SetLocale(demoLocale)
	}
	if email != "" && (u.Email == nil || *u.Email == "") {
		// Do not steal another account's email address. Leaving it unset is
		// safer than failing startup on a shared host.
		taken, qErr := client.User.Query().
			Where(user.EmailEQ(email), user.IDNEQ(u.ID)).
			Exist(ctx)
		if qErr != nil {
			return fmt.Errorf("check demo email: %w", qErr)
		}
		if !taken {
			update.SetEmail(email)
		}
	}
	u, err = update.Save(ctx)
	if err != nil {
		return fmt.Errorf("repair demo user: %w", err)
	}

	return seedDemoContent(ctx, client, u, now)
}

func createDemoUser(ctx context.Context, client *ent.Client, handle, email, password string, now time.Time) (*ent.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	create := client.User.Create().
		SetUsername(handle).
		SetPasswordHash(string(hash)).
		SetRole(user.RoleUser).
		SetLocale(demoLocale).
		SetCreatedAt(now).
		SetUpdatedAt(now)
	if email != "" {
		taken, qErr := client.User.Query().Where(user.EmailEQ(email)).Exist(ctx)
		if qErr != nil {
			return nil, qErr
		}
		if !taken {
			create.SetEmail(email)
		}
	}
	return create.Save(ctx)
}

// seedDemoContent populates a realistic relationship-reminder dataset. It
// never seeds when any people already exist: People are shared across the
// whole instance, so an existing address book must not be polluted.
func seedDemoContent(ctx context.Context, client *ent.Client, u *ent.User, now time.Time) error {
	count, err := client.Person.Query().Count(ctx)
	if err != nil {
		return fmt.Errorf("count people: %w", err)
	}
	if count > 0 {
		return nil
	}

	family, err := ensureGroup(ctx, client, "Family", "Immediate family and close relatives.", now)
	if err != nil {
		return err
	}
	partnerTag, err := ensureTag(ctx, client, "partner", now)
	if err != nil {
		return err
	}
	friendTag, err := ensureTag(ctx, client, "friend", now)
	if err != nil {
		return err
	}

	alex, err := createPerson(ctx, client, "Alex Morgan", "Alex", "", "Morgan", alexNotes, now)
	if err != nil {
		return err
	}
	sam, err := createPerson(ctx, client, "Sam Rivera", "Sam", "", "Rivera", samNotes, now)
	if err != nil {
		return err
	}
	jordan, err := createPerson(ctx, client, "Jordan Lee", "Jordan", "", "Lee", jordanNotes, now)
	if err != nil {
		return err
	}

	if err := client.Person.UpdateOneID(alex.ID).AddGroupIDs(family.ID).AddTagIDs(partnerTag.ID).Exec(ctx); err != nil {
		return fmt.Errorf("link Alex: %w", err)
	}
	if err := client.Person.UpdateOneID(sam.ID).AddTagIDs(friendTag.ID).Exec(ctx); err != nil {
		return fmt.Errorf("link Sam: %w", err)
	}
	if err := client.Person.UpdateOneID(jordan.ID).AddGroupIDs(family.ID).AddTagIDs(friendTag.ID).Exec(ctx); err != nil {
		return fmt.Errorf("link Jordan: %w", err)
	}

	rels := []struct {
		from, to int
		typ      relationship.Type
		label    string
	}{
		{alex.ID, jordan.ID, relationship.TypePartner, "partner"},
		{alex.ID, sam.ID, relationship.TypeCustom, "friend"},
		{jordan.ID, sam.ID, relationship.TypeCustom, "friend"},
	}
	for _, rel := range rels {
		if _, err := client.Relationship.Create().
			SetFromID(rel.from).
			SetToID(rel.to).
			SetType(rel.typ).
			SetLabel(rel.label).
			Save(ctx); err != nil {
			return fmt.Errorf("seed relationship: %w", err)
		}
	}

	events := []struct {
		personID int
		typ      string
		date     time.Time
		desc     string
		reminder []int
	}{
		{alex.ID, "birthday", birthdayInMonths(now, 2, 14), "Alex's birthday", []int{14, 3}},
		{alex.ID, "anniversary", dateInMonths(now, 5, 8), "Alex and Jordan's anniversary", []int{7}},
		{jordan.ID, "birthday", birthdayInMonths(now, 8, 21), "Jordan's birthday", []int{14, 3}},
		{sam.ID, "custom", dateInMonths(now, 10, 12), "Sam's housewarming party", []int{3}},
	}
	for _, e := range events {
		if _, err := client.Event.Create().
			SetType(e.typ).
			SetDate(e.date).
			SetDescription(e.desc).
			SetReminderDays(e.reminder).
			SetCalendarSystem("gregorian").
			SetCreatedAt(now).
			SetPersonID(e.personID).
			Save(ctx); err != nil {
			return fmt.Errorf("seed event %q: %w", e.desc, err)
		}
	}

	gifts := []struct {
		personID int
		title    string
		notes    string
		price    int
	}{
		{alex.ID, "Trail running shoes", "Size 42, neutral cushioning; the blue pair.", 12000},
		{sam.ID, "Coffee subscription", "Three-month single-origin subscription.", 4500},
	}
	for _, g := range gifts {
		if _, err := client.GiftIdea.Create().
			SetTitle(g.title).
			SetNotes(g.notes).
			SetPriceCents(g.price).
			SetStatus(giftidea.StatusIdea).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			SetPersonID(g.personID).
			Save(ctx); err != nil {
			return fmt.Errorf("seed gift %q: %w", g.title, err)
		}
	}

	notes := []struct {
		personID int
		note     string
		date     time.Time
	}{
		{alex.ID, "Prefers tea over coffee after 6pm.", now.AddDate(0, -1, 0)},
		{sam.ID, "Moved to Lisbon last spring; ask about the new job.", now.AddDate(0, -2, 0)},
	}
	for _, n := range notes {
		if _, err := client.PersonNote.Create().
			SetNote(n.note).
			SetNoteDate(n.date).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			SetPersonID(n.personID).
			Save(ctx); err != nil {
			return fmt.Errorf("seed person note: %w", err)
		}
	}

	// Link the demo user to the partner so their own dashboard has a face.
	if u.LinkedPersonID == nil {
		if _, err := client.User.UpdateOneID(u.ID).
			SetLinkedPersonID(alex.ID).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return fmt.Errorf("link demo person: %w", err)
		}
	}

	return nil
}

func createPerson(ctx context.Context, client *ent.Client, display, first, middle, last, notes string, now time.Time) (*ent.Person, error) {
	create := client.Person.Create().
		SetName(display).
		SetFirstName(first).
		SetLastName(last).
		SetNotes(notes).
		SetCreatedAt(now).
		SetUpdatedAt(now)
	if middle != "" {
		create.SetMiddleName(middle)
	}
	p, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed person %q: %w", display, err)
	}
	return p, nil
}

func ensureGroup(ctx context.Context, client *ent.Client, name, description string, now time.Time) (*ent.Group, error) {
	existing, err := client.Group.Query().Where(group.NameEQ(name)).Only(ctx)
	if err == nil {
		return existing, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("find group %q: %w", name, err)
	}
	g, err := client.Group.Create().
		SetName(name).
		SetDescription(description).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed group %q: %w", name, err)
	}
	return g, nil
}

func ensureTag(ctx context.Context, client *ent.Client, name string, now time.Time) (*ent.Tag, error) {
	existing, err := client.Tag.Query().Where(tag.NameEQ(name)).Only(ctx)
	if err == nil {
		return existing, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("find tag %q: %w", name, err)
	}
	t, err := client.Tag.Create().
		SetName(name).
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed tag %q: %w", name, err)
	}
	return t, nil
}

func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}
	return strings.EqualFold(addr.Address, s)
}

// dateInMonths returns a UTC date roughly monthsAhead months from now, on the
// given day of that month.
func dateInMonths(now time.Time, monthsAhead, day int) time.Time {
	d := now.AddDate(0, monthsAhead, 0)
	if day < 1 {
		day = 1
	}
	for day > daysInMonth(d.Year(), d.Month()) {
		day--
	}
	return time.Date(d.Year(), d.Month(), day, 0, 0, 0, 0, time.UTC)
}

// birthdayInMonths builds a stored birth date whose month/day falls in the
// next 12 months, so it behaves like a real birthday that recurs annually.
func birthdayInMonths(now time.Time, monthsAhead, day int) time.Time {
	d := dateInMonths(now, monthsAhead, day)
	return time.Date(d.Year()-30, d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
