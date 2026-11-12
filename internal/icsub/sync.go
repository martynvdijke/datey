package icsub

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/ics"
	"github.com/datey/datey/internal/repository"
)

// eventType is the Datey event type assigned to feed events. They are not
// birthdays or anniversaries, so the generic "custom" type fits.
const eventType = "custom"

// Summary reports what one sync pass changed.
type Summary struct {
	Created int
	Updated int
	Deleted int
}

// Syncer applies a remote feed to the events owned by one subscription.
type Syncer struct {
	subs    *repository.SubscriptionRepository
	events  *repository.EventRepository
	fetcher Fetcher
	now     func() time.Time
}

// NewSyncer builds a syncer for client. A nil fetcher uses HTTPFetcher.
func NewSyncer(client *ent.Client, fetcher Fetcher) *Syncer {
	if fetcher == nil {
		fetcher = NewHTTPFetcher()
	}
	return &Syncer{
		subs:    repository.NewSubscriptionRepository(client),
		events:  repository.NewEventRepository(client),
		fetcher: fetcher,
		now:     time.Now,
	}
}

// SyncID looks up a subscription and syncs it.
func (s *Syncer) SyncID(ctx context.Context, id int) (Summary, error) {
	sub, err := s.subs.Get(ctx, id)
	if err != nil {
		return Summary{}, fmt.Errorf("load subscription %d: %w", id, err)
	}
	return s.Sync(ctx, sub)
}

// Sync fetches sub's feed and applies the UID-keyed diff to the events that
// belong to that subscription. A fetch or parse failure records the attempt and
// returns without touching any event (D3: the last good state survives
// outages).
func (s *Syncer) Sync(ctx context.Context, sub *ent.Subscription) (Summary, error) {
	var summary Summary
	attemptedAt := s.now()

	feed, err := s.fetcher.Fetch(ctx, sub.URL)
	if err != nil {
		s.record(ctx, sub.ID, attemptedAt, false, err.Error())
		return summary, fmt.Errorf("sync subscription %d: %w", sub.ID, err)
	}

	// Only events with a UID can be diffed. Duplicate UIDs are resolved
	// deterministically by keeping the first occurrence in feed order.
	desired := make(map[string]ics.Event, len(feed))
	for _, fe := range feed {
		if fe.UID == "" {
			continue
		}
		if _, exists := desired[fe.UID]; exists {
			continue
		}
		desired[fe.UID] = fe
	}

	local, err := s.subs.EventsBySubscription(ctx, sub.ID)
	if err != nil {
		s.record(ctx, sub.ID, attemptedAt, false, err.Error())
		return summary, fmt.Errorf("list subscription %d events: %w", sub.ID, err)
	}
	localByUID := make(map[string]*ent.Event, len(local))
	for _, e := range local {
		if e.ExternalUID != "" {
			localByUID[e.ExternalUID] = e
		}
	}

	for uid, fe := range desired {
		existing, ok := localByUID[uid]
		if !ok {
			if _, err := s.events.CreateForSubscription(ctx, sub.ID, uid, eventType, fe.Start, fe.Summary); err != nil {
				s.record(ctx, sub.ID, attemptedAt, false, err.Error())
				return summary, fmt.Errorf("create event %q: %w", uid, err)
			}
			summary.Created++
			continue
		}
		if !eventChanged(existing, fe) {
			continue
		}
		if _, err := s.events.UpdateForSubscription(ctx, existing.ID, eventType, fe.Start, fe.Summary); err != nil {
			s.record(ctx, sub.ID, attemptedAt, false, err.Error())
			return summary, fmt.Errorf("update event %q: %w", uid, err)
		}
		summary.Updated++
	}

	// Deletion is guarded: a feed that yielded no UID at all is treated as
	// unusable rather than as "everything disappeared", so a malformed or
	// UID-less provider can never blank a calendar.
	if len(desired) > 0 {
		for uid, e := range localByUID {
			if _, ok := desired[uid]; ok {
				continue
			}
			if err := s.events.Delete(ctx, e.ID); err != nil {
				s.record(ctx, sub.ID, attemptedAt, false, err.Error())
				return summary, fmt.Errorf("delete event %q: %w", uid, err)
			}
			summary.Deleted++
		}
	}

	s.record(ctx, sub.ID, attemptedAt, true, "")
	return summary, nil
}

// SyncAllEnabled syncs every enabled subscription, continuing past individual
// failures so one unreachable provider does not block the others.
func (s *Syncer) SyncAllEnabled(ctx context.Context) (Summary, error) {
	subs, err := s.subs.ListEnabled(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("list enabled subscriptions: %w", err)
	}
	var total Summary
	for _, sub := range subs {
		summary, err := s.Sync(ctx, sub)
		total.Created += summary.Created
		total.Updated += summary.Updated
		total.Deleted += summary.Deleted
		if err != nil {
			slog.Warn("subscription sync failed", "source", "icsub", "subscription", sub.ID, "error", err)
		}
	}
	return total, nil
}

// record persists the attempt outcome. A failure to record is logged but does
// not mask the sync result.
func (s *Syncer) record(ctx context.Context, id int, attemptedAt time.Time, success bool, errMsg string) {
	if err := s.subs.UpdateFetchStatus(ctx, id, attemptedAt, success, errMsg); err != nil {
		slog.Error("record subscription fetch status", "source", "icsub", "subscription", id, "error", err)
	}
}

// eventChanged reports whether the feed-derived fields differ from the stored
// event. Ownership and user-editable notes are intentionally excluded.
func eventChanged(e *ent.Event, fe ics.Event) bool {
	if e.Description != fe.Summary {
		return true
	}
	if e.Type != eventType {
		return true
	}
	return !e.Date.Equal(fe.Start)
}
