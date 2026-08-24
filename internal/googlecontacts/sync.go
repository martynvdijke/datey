package googlecontacts

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/settings"
)

// Result summarizes a sync.
type Result struct {
	Created int
	Updated int
	Deleted int
	Skipped int
	Errors  []string
}

type Syncer struct {
	cfg      *config.Config
	client   *ent.Client
	settings *settings.Store
	people   *repository.PersonRepository
	events   *repository.EventRepository
	gc       *Client
}

func NewSyncer(cfg *config.Config, client *ent.Client, store *settings.Store, gc *Client) *Syncer {
	return &Syncer{
		cfg:      cfg,
		client:   client,
		settings: store,
		people:   repository.NewPersonRepository(client),
		events:   repository.NewEventRepository(client),
		gc:       gc,
	}
}

func NewSyncerWithClient(cfg *config.Config, client *ent.Client, store *settings.Store, gc *Client) *Syncer {
	return NewSyncer(cfg, client, store, gc)
}

// Sync pulls remote contacts and upserts local people.
func (s *Syncer) Sync(ctx context.Context) (*Result, error) {
	res := &Result{}
	syncToken, err := s.settings.GoogleSyncToken(ctx)
	if err != nil {
		return res, fmt.Errorf("google sync token: %w", err)
	}
	// If delete policy is delete, force full sync to reconcile deletions
	if s.cfg.GoogleDeletePolicy == "delete" {
		syncToken = ""
	}
	// A full fetch is required before absence-based reconciliation: an
	// incremental (syncToken) response lists only changed contacts, so a
	// locally-synced person missing from it was NOT necessarily deleted
	// remotely. Explicit tombstones (metadata.deleted) are handled below.
	fullSync := syncToken == ""
	listRes, err := s.gc.ListContacts(ctx, syncToken)
	if err != nil {
		return res, err
	}
	remoteNames := map[string]bool{}
	for _, c := range listRes.Contacts {
		if c.Metadata != nil && c.Metadata.Deleted {
			s.applyDeletionPolicy(ctx, c.ResourceName, res)
			continue
		}
		remoteNames[c.ResourceName] = true
		if err := s.upsertContact(ctx, c, res); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", c.ResourceName, err))
		}
	}
	// Handle deletions: contacts absent remotely that were previously synced.
	if fullSync {
		if err := s.handleDeletions(ctx, remoteNames, res); err != nil {
			res.Errors = append(res.Errors, err.Error())
		}
	}
	if listRes.NextSyncToken != "" {
		if err := s.settings.SetGoogleSyncToken(ctx, listRes.NextSyncToken); err != nil {
			slog.Error("google: persist sync token", "error", err)
		}
		s.cfg.GoogleSyncToken = listRes.NextSyncToken
	}
	if err := s.settings.SetGoogleLastSync(ctx, time.Now()); err != nil {
		slog.Error("google: persist last sync", "error", err)
	}
	slog.Info("google: sync complete", "created", res.Created, "updated", res.Updated, "deleted", res.Deleted, "skipped", res.Skipped)
	return res, nil
}

func (s *Syncer) upsertContact(ctx context.Context, c Contact, res *Result) error {
	name := c.DisplayName()
	if name == "" {
		res.Skipped++
		return nil
	}
	birthday := c.BirthdayTime()
	notes := c.BiographyText()

	// Try match by resource name
	person, err := s.people.FindByGoogleResourceName(ctx, c.ResourceName)
	if err == nil {
		// Update
		if err := s.updatePerson(ctx, person, name, birthday, notes, c.ResourceName, res); err != nil {
			return err
		}
		res.Updated++
		return nil
	}
	if !ent.IsNotFound(err) {
		return err
	}
	// Fallback: exact name + matching birthday
	if birthday != nil {
		if p2, err := s.people.FindByName(ctx, name); err == nil {
			// check birthday matches
			if bday := s.personBirthday(ctx, p2.ID); bday != nil && bday.Equal(*birthday) {
				// Link and update
				if err := s.updatePerson(ctx, p2, name, birthday, notes, c.ResourceName, res); err != nil {
					return err
				}
				res.Updated++
				return nil
			}
		}
	}
	// Create new person; spec normative says still create without birthday event.
	p, err := s.people.Create(ctx, name, notes, "")
	if err != nil {
		return err
	}
	// Set google resource name
	if _, err := s.people.SetGoogleResourceName(ctx, p.ID, c.ResourceName); err != nil {
		return err
	}
	if birthday != nil {
		if err := s.upsertBirthdayEvent(ctx, p.ID, *birthday, name); err != nil {
			return err
		}
	}
	res.Created++
	return nil
}

func (s *Syncer) updatePerson(ctx context.Context, p *ent.Person, name string, birthday *time.Time, notes, resourceName string, res *Result) error {
	// Update name/notes if changed
	if p.Name != name || p.Notes != notes {
		if _, err := s.people.UpdateGoogleFields(ctx, p.ID, name, notes); err != nil {
			return err
		}
	}
	if _, err := s.people.SetGoogleResourceName(ctx, p.ID, resourceName); err != nil {
		return err
	}
	if birthday != nil {
		if err := s.upsertBirthdayEvent(ctx, p.ID, *birthday, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Syncer) personBirthday(ctx context.Context, personID int) *time.Time {
	events, err := s.events.ListByPerson(ctx, personID)
	if err != nil {
		return nil
	}
	for _, ev := range events {
		if ev.Type == "birthday" {
			t := ev.Date
			return &t
		}
	}
	return nil
}

func (s *Syncer) upsertBirthdayEvent(ctx context.Context, personID int, birthday time.Time, name string) error {
	events, err := s.events.ListByPerson(ctx, personID)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.Type == "birthday" {
			if !ev.Date.Equal(birthday) {
				if _, err := s.events.Update(ctx, ev.ID, "birthday", birthday, ev.Description); err != nil {
					return err
				}
			}
			return nil
		}
	}
	_, err = s.events.CreateForPerson(ctx, personID, "birthday", birthday, "Birthday of "+name)
	return err
}

func (s *Syncer) handleDeletions(ctx context.Context, remote map[string]bool, res *Result) error {
	// List all local people with google_resource_name
	all, err := s.people.ListGoogleSynced(ctx)
	if err != nil {
		return err
	}
	for _, p := range all {
		if p.GoogleResourceName == nil {
			continue
		}
		if !remote[*p.GoogleResourceName] {
			s.applyDeletionPolicy(ctx, *p.GoogleResourceName, res)
		}
	}
	return nil
}

// applyDeletionPolicy reconciles one remotely-gone contact against its local
// person: default policy unlinks and keeps, strict policy removes the person
// and their events.
func (s *Syncer) applyDeletionPolicy(ctx context.Context, resourceName string, res *Result) {
	p, err := s.people.FindByGoogleResourceName(ctx, resourceName)
	if err != nil || p == nil {
		return // nothing linked locally
	}
	if s.cfg.GoogleDeletePolicy == "delete" {
		events, _ := s.events.ListByPerson(ctx, p.ID)
		for _, ev := range events {
			_ = s.events.Delete(ctx, ev.ID)
		}
		if err := s.people.Delete(ctx, p.ID); err != nil {
			res.Errors = append(res.Errors, err.Error())
			return
		}
	} else if _, err := s.people.ClearGoogleResourceName(ctx, p.ID); err != nil {
		res.Errors = append(res.Errors, err.Error())
		return
	}
	res.Deleted++
}
