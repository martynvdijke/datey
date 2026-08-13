package carddav

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/settings"
	"github.com/datey/datey/internal/vcard"
)

// SyncMode selects the direction of a manual or scheduled sync.
type SyncMode int

const (
	// SyncFull pulls remote changes then pushes local changes.
	SyncFull SyncMode = iota
	// SyncPull only applies remote changes.
	SyncPull
	// SyncPush only applies local changes.
	SyncPush
)

func (m SyncMode) String() string {
	switch m {
	case SyncFull:
		return "full"
	case SyncPull:
		return "pull"
	case SyncPush:
		return "push"
	}
	return "unknown"
}

// Result summarizes a completed sync for logging.
type Result struct {
	Mode             string
	PulledCreated    int
	PulledUpdated    int
	PulledDeleted    int
	PushedCreated    int
	PushedUpdated    int
	PushedDeleted    int
	Errors           []string
}

func (r *Result) AddError(format string, args ...any) {
	r.Errors = append(r.Errors, fmt.Sprintf(format, args...))
}

// Syncer drives a two-way CardDAV sync between the remote address book and the
// local people/events model. Conflicts are resolved last-write-wins: a remote
// REV timestamp newer than the local person's UpdatedAt wins over a pending
// local edit; otherwise a pending local edit is kept (and pushed later).
type Syncer struct {
	cfg      *config.Config
	client   *ent.Client
	settings *settings.Store
	people   *repository.PersonRepository
	events   *repository.EventRepository
	carddav  *Client
	bookURL  string
	// manual marks a user-invoked sync; it skips the daily rate limit.
	manual bool
}

// NewSyncer builds a Syncer bound to the address book URL in cfg. The URL is
// resolved via discovery on first use; when the configured URL already points
// at a collection it is used as-is.
func NewSyncer(cfg *config.Config, client *ent.Client, store *settings.Store) *Syncer {
	transport := &BasicAuthTransport{
		Username: cfg.CarddavUsername,
		Password: cfg.CarddavPassword,
	}
	return &Syncer{
		cfg:      cfg,
		client:   client,
		settings: store,
		people:   repository.NewPersonRepository(client),
		events:   repository.NewEventRepository(client),
		carddav:  New(cfg.CarddavURL, transport),
	}
}

// Sync runs the requested sync mode and records the outcome in the log store.
func (s *Syncer) Sync(ctx context.Context, mode SyncMode, manual bool) (*Result, error) {
	s.manual = manual
	res := &Result{Mode: mode.String()}

	if s.cfg.CarddavURL == "" {
		return nil, fmt.Errorf("carddav: no address book URL configured")
	}

	// Resolve the collection URL once per sync (cached on the Syncer).
	if s.bookURL == "" {
		bookURL, err := s.carddav.Discover(ctx)
		if err != nil {
			// The configured URL may already be a collection that does not
			// answer discovery; fall back to using it directly.
			s.bookURL = s.cfg.CarddavURL
			s.carddav = New(s.bookURL, &BasicAuthTransport{
				Username: s.cfg.CarddavUsername,
				Password: s.cfg.CarddavPassword,
			})
			slog.Warn("carddav: discovery failed, using configured URL", "url", s.bookURL, "error", err)
		} else {
			s.bookURL = bookURL
			s.carddav = New(s.bookURL, &BasicAuthTransport{
				Username: s.cfg.CarddavUsername,
				Password: s.cfg.CarddavPassword,
			})
			slog.Info("carddav: address book resolved", "url", s.bookURL)
		}
	}

	switch mode {
	case SyncPull, SyncFull:
		if err := s.pull(ctx, res); err != nil {
			return res, err
		}
	}
	switch mode {
	case SyncPush, SyncFull:
		if err := s.push(ctx, res); err != nil {
			return res, err
		}
	}

	if err := s.settings.SetCarddavLastSync(ctx, time.Now()); err != nil {
		slog.Error("carddav: persist last sync", "error", err)
	}

	if len(res.Errors) > 0 {
		for _, e := range res.Errors {
			slog.Error("carddav: sync error", "error", e)
		}
	}
	slog.Info("carddav: sync complete",
		"mode", res.Mode,
		"pulled_created", res.PulledCreated,
		"pulled_updated", res.PulledUpdated,
		"pulled_deleted", res.PulledDeleted,
		"pushed_created", res.PushedCreated,
		"pushed_updated", res.PushedUpdated,
		"pushed_deleted", res.PushedDeleted,
		"errors", len(res.Errors),
	)

	return res, nil
}

// pull applies remote changes. With the default keep-deletion policy an
// incremental sync-collection REPORT is used; with the delete policy a full
// report is requested so that locally deleted contacts can be reconciled.
func (s *Syncer) pull(ctx context.Context, res *Result) error {
	token, err := s.settings.CarddavSyncToken(ctx)
	if err != nil {
		return fmt.Errorf("carddav: read sync token: %w", err)
	}
	if s.cfg.CarddavDeletePolicy == "delete" {
		token = "" // full report: needed to see all remote hrefs
	}

	report, err := s.carddav.SyncCollection(ctx, token)
	if err != nil {
		return err
	}

	remoteHrefs := map[string]bool{}
	for _, item := range report.Responses {
		if strings.Contains(item.Status, "404") {
			if err := s.applyRemoteDeletion(ctx, item.Href, res); err != nil {
				res.AddError("delete %s: %v", item.Href, err)
			}
			continue
		}
		remoteHrefs[item.Href] = true
		if err := s.applyRemoteChange(ctx, item.Href, res); err != nil {
			res.AddError("pull %s: %v", item.Href, err)
		}
	}

	// Reconcile local deletions: remote hrefs that no longer exist locally
	// were deleted from Datey and must be removed from the remote book when
	// the delete policy is enabled (full report only).
	if s.cfg.CarddavDeletePolicy == "delete" {
		if err := s.reconcileRemoteDeletions(ctx, remoteHrefs, res); err != nil {
			res.AddError("reconcile deletions: %v", err)
		}
	}

	if report.SyncToken != "" {
		if err := s.settings.SetCarddavSyncToken(ctx, report.SyncToken); err != nil {
			slog.Error("carddav: persist sync token", "error", err)
		}
	}
	return nil
}

// applyRemoteChange fetches a changed vCard, parses it, and upserts the local
// person matched by UID (falling back to a name match for servers that omit
// UIDs). New contacts get a birthday event when the vCard has a BDAY.
func (s *Syncer) applyRemoteChange(ctx context.Context, href string, res *Result) error {
	data, err := s.carddav.Get(ctx, href)
	if err != nil {
		return err
	}
	pc, err := s.parseSingle(data)
	if err != nil || pc.Name == "" {
		return fmt.Errorf("parse vCard: %w", err)
	}

	// Match by UID first (design D4); name is a fallback for UID-less cards.
	person, err := s.findMatch(ctx, pc)
	if err != nil {
		return err
	}

	if person == nil {
		// Brand-new remote contact.
		p, err := s.people.Create(ctx, pc.Name, pc.Notes, string(data))
		if err != nil {
			return fmt.Errorf("create person %q: %w", pc.Name, err)
		}
		if _, err := s.people.SetCarddavState(ctx, p.ID, pc.UID, href, "", pc.Rev, nil, false); err != nil {
			return err
		}
		if err := s.upsertBirthdayEvent(ctx, p.ID, pc); err != nil {
			return err
		}
		res.PulledCreated++
		return nil
	}

	// Conflict resolution: a pending local edit wins unless the remote REV is
	// provably newer than the local change (last-write-wins, design D3).
	if person.CarddavPendingSync {
		remoteNewer, err := s.remoteIsNewer(pc, person)
		if err == nil && !remoteNewer {
			slog.Debug("carddav: keeping pending local change", "person", person.ID)
			return nil
		}
	}

	if _, err := s.people.UpdateCarddavFields(ctx, person.ID, pc.Name, pc.Notes, string(data)); err != nil {
		return err
	}
	if _, err := s.people.SetCarddavState(ctx, person.ID, pc.UID, href, "", pc.Rev, nil, false); err != nil {
		return err
	}
	if err := s.upsertBirthdayEvent(ctx, person.ID, pc); err != nil {
		return err
	}
	res.PulledUpdated++
	return nil
}

// findMatch locates the local person corresponding to a remote vCard. UID
// matches first; when the vCard has no UID the contact name is used.
func (s *Syncer) findMatch(ctx context.Context, pc vcard.ParsedContact) (*ent.Person, error) {
	if pc.UID != "" {
		p, err := s.people.FindByCarddavUID(ctx, pc.UID)
		if err == nil {
			return p, nil
		}
		if !ent.IsNotFound(err) {
			return nil, err
		}
	}
	p, err := s.people.FindByName(ctx, pc.Name)
	if err == nil {
		return p, nil
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}
	return nil, nil
}

// remoteIsNewer reports whether the remote vCard REV is a parseable RFC3339
// timestamp strictly after the local person's last modification. Unparseable
// REVs are treated as "remote wins" (design: unparseable/absent → remote).
func (s *Syncer) remoteIsNewer(pc vcard.ParsedContact, p *ent.Person) (bool, error) {
	if pc.Rev == "" {
		return true, nil
	}
	rev, err := time.Parse(time.RFC3339, pc.Rev)
	if err != nil {
		return true, nil
	}
	return rev.After(p.UpdatedAt), nil
}

// applyRemoteDeletion handles a remote-deleted contact. With the keep policy
// the local person is preserved and unlinked; with the delete policy the local
// person (and their events) is removed.
func (s *Syncer) applyRemoteDeletion(ctx context.Context, href string, res *Result) error {
	p, err := s.people.FindByCarddavHref(ctx, href)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil // already gone locally
		}
		return err
	}

	if s.cfg.CarddavDeletePolicy == "keep" {
		// Keep the local person, drop the remote linkage so the next sync
		// does not try to update a resource that no longer exists.
		_, err := s.people.ClearCarddavState(ctx, p.ID)
		return err
	}

	if err := s.deleteLocalPerson(ctx, p.ID); err != nil {
		return err
	}
	res.PulledDeleted++
	return nil
}

// reconcileRemoteDeletions removes remote resources whose hrefs no longer
// correspond to any synced local person (i.e. the contact was deleted in
// Datey). Only runs for the delete policy, which requests full reports.
func (s *Syncer) reconcileRemoteDeletions(ctx context.Context, remoteHrefs map[string]bool, res *Result) error {
	local, err := s.people.ListCarddavSynced(ctx)
	if err != nil {
		return err
	}
	localHrefs := map[string]bool{}
	for _, p := range local {
		if p.CarddavHref != nil {
			localHrefs[*p.CarddavHref] = true
		}
	}
	for href := range remoteHrefs {
		if !localHrefs[href] {
			deleted, err := s.carddav.Delete(ctx, href)
			if err != nil {
				res.AddError("delete remote %s: %v", href, err)
				continue
			}
			if deleted {
				res.PushedDeleted++
			}
		}
	}
	return nil
}

// push applies local changes to the remote address book. People without a UID
// are created remotely (new href, generated UID); synced people with pending
// changes are updated with an If-Match guard using the stored ETag.
func (s *Syncer) push(ctx context.Context, res *Result) error {
	candidates, err := s.people.ListForCarddavPush(ctx)
	if err != nil {
		return err
	}

	for _, p := range candidates {
		if p.CarddavUID == nil {
			if err := s.pushCreate(ctx, p, res); err != nil {
				res.AddError("push create %q: %v", p.Name, err)
			}
			continue
		}
		if err := s.pushUpdate(ctx, p, res); err != nil {
			res.AddError("push update %q: %v", p.Name, err)
		}
	}
	return nil
}

// pushCreate writes a new vCard to the remote book and records the assigned
// UID/href/ETag on the local person.
func (s *Syncer) pushCreate(ctx context.Context, p *ent.Person, res *Result) error {
	uid := newUID()
	href := strings.TrimRight(s.bookURL, "/") + "/" + uid + ".vcf"
	data, err := s.buildPushVCard(ctx, p, uid)
	if err != nil {
		return err
	}
	put, err := s.carddav.Put(ctx, href, data, "")
	if err != nil {
		return err
	}
	if _, err := s.people.SetCarddavState(ctx, p.ID, uid, href, put.ETag, "", nil, false); err != nil {
		return err
	}
	res.PushedCreated++
	return nil
}

// pushUpdate writes local changes back to an existing remote resource,
// guarded by If-Match against the stored ETag to avoid clobbering a remote
// change that arrived after the last pull.
func (s *Syncer) pushUpdate(ctx context.Context, p *ent.Person, res *Result) error {
	href := p.CarddavHref
	if href == nil || *href == "" {
		href = nillableString(strings.TrimRight(s.bookURL, "/") + "/" + *p.CarddavUID + ".vcf")
	}
	etag := ""
	if p.CarddavEtag != nil {
		etag = *p.CarddavEtag
	}
	data, err := s.buildPushVCard(ctx, p, *p.CarddavUID)
	if err != nil {
		return err
	}
	put, err := s.carddav.Put(ctx, *href, data, etag)
	if err != nil {
		// A 412 means the remote changed since our last pull; the next pull
		// will reconcile. Keep the person pending so the change is not lost.
		return err
	}
	if _, err := s.people.SetCarddavState(ctx, p.ID, *p.CarddavUID, *href, put.ETag, "", nil, false); err != nil {
		return err
	}
	res.PushedUpdated++
	return nil
}

// buildPushVCard assembles the vCard payload for a local person, including
// the stored provider UID, the birthday event (when one exists) and notes.
func (s *Syncer) buildPushVCard(ctx context.Context, p *ent.Person, uid string) ([]byte, error) {
	birthday, err := s.findBirthday(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return vcard.EncodeContact(vcard.SyncContact{
		Name:     p.Name,
		Notes:    p.Notes,
		Birthday: birthday,
		UID:      uid,
	})
}

// upsertBirthdayEvent ensures a local birthday event matches the vCard BDAY:
// created when missing, updated when the date differs.
func (s *Syncer) upsertBirthdayEvent(ctx context.Context, personID int, pc vcard.ParsedContact) error {
	if pc.Birthday == nil {
		return nil
	}
	events, err := s.events.ListByPerson(ctx, personID)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.Type == "birthday" {
			if !ev.Date.Equal(*pc.Birthday) {
				if _, err := s.events.Update(ctx, ev.ID, "birthday", *pc.Birthday, ev.Description); err != nil {
					return err
				}
			}
			return nil
		}
	}
	_, err = s.events.CreateForPerson(ctx, personID, "birthday", *pc.Birthday, "Birthday of "+pc.Name)
	return err
}

// findBirthday returns the birthday event date for a person, or nil.
func (s *Syncer) findBirthday(ctx context.Context, personID int) (*time.Time, error) {
	events, err := s.events.ListByPerson(ctx, personID)
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		if ev.Type == "birthday" {
			t := ev.Date
			return &t, nil
		}
	}
	return nil, nil
}

// deleteLocalPerson removes a person and their events (the ent schema has no
// ON DELETE cascade for person→event).
func (s *Syncer) deleteLocalPerson(ctx context.Context, personID int) error {
	events, err := s.events.ListByPerson(ctx, personID)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if err := s.events.Delete(ctx, ev.ID); err != nil {
			return err
		}
	}
	return s.people.Delete(ctx, personID)
}

// parseSingle parses a single vCard resource body into a ParsedContact.
func (s *Syncer) parseSingle(data []byte) (vcard.ParsedContact, error) {
	contacts, err := vcard.Parse(strings.NewReader(string(data)))
	if err != nil {
		return vcard.ParsedContact{}, err
	}
	if len(contacts) == 0 {
		return vcard.ParsedContact{}, fmt.Errorf("no vCard entries")
	}
	return contacts[0], nil
}

// newUID returns a random hex identifier for a new remote vCard.
func newUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func nillableString(s string) *string { return &s }
