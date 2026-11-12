package repository

import (
	"context"
	"log/slog"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/channelhealth"
)

// channelHealthKeySeparator joins a channel name and its target into a single
// map key. Channel names and targets are user/config supplied, so a control
// character keeps the two parts from colliding.
const channelHealthKeySeparator = "\x1f"

// ChannelHealthRepository persists per-channel delivery health. It uses ent
// builders only, like every other repository in this package.
type ChannelHealthRepository struct {
	client *ent.Client
	// Now is the clock used for the recorded timestamps. It is nil in
	// production (time.Now is used) and injectable in tests.
	Now func() time.Time
}

func NewChannelHealthRepository(client *ent.Client) *ChannelHealthRepository {
	return &ChannelHealthRepository{client: client}
}

func (r *ChannelHealthRepository) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// ChannelHealthKey builds the lookup key shared by the repository and the web
// layer so both address a health row by exactly the same (channel, target)
// pair that the unique index uses.
func ChannelHealthKey(channel, target string) string {
	return channel + channelHealthKeySeparator + target
}

// IndexChannelHealth turns a batch of health rows into a lookup map. Callers
// load every row once (ListAll) and then index into the result, which keeps the
// web layer free of per-channel queries.
func IndexChannelHealth(rows []*ent.ChannelHealth) map[string]*ent.ChannelHealth {
	idx := make(map[string]*ent.ChannelHealth, len(rows))
	for _, row := range rows {
		idx[ChannelHealthKey(row.Channel, row.Target)] = row
	}
	return idx
}

// Upsert records the outcome of one delivery attempt. A nil err is a success.
func (r *ChannelHealthRepository) Upsert(ctx context.Context, channel, target string, sendErr error) error {
	if sendErr == nil {
		return r.RecordSuccess(ctx, channel, target)
	}
	return r.RecordFailure(ctx, channel, target, sendErr.Error())
}

// RecordSuccess marks a successful send: it stamps last_success_at, clears the
// consecutive failure counter and drops the stale error message so the UI does
// not keep showing a failure that has since been resolved.
func (r *ChannelHealthRepository) RecordSuccess(ctx context.Context, channel, target string) error {
	now := r.now()
	return r.upsert(ctx, channel, target, func(b *ent.ChannelHealthUpdateOne) {
		b.SetLastSuccessAt(now).SetConsecutiveFailures(0).SetLastError("")
	}, func(c *ent.ChannelHealthCreate) {
		c.SetLastSuccessAt(now).SetConsecutiveFailures(0)
	})
}

// RecordFailure marks a failed send: it stamps last_failure_at, increments the
// consecutive failure counter and stores the (already truncated) message.
func (r *ChannelHealthRepository) RecordFailure(ctx context.Context, channel, target, message string) error {
	now := r.now()
	return r.upsert(ctx, channel, target, func(b *ent.ChannelHealthUpdateOne) {
		b.SetLastFailureAt(now).AddConsecutiveFailures(1).SetLastError(message)
	}, func(c *ent.ChannelHealthCreate) {
		c.SetLastFailureAt(now).SetConsecutiveFailures(1).SetLastError(message)
	})
}

// upsert applies update to an existing (channel, target) row or creates one.
// A concurrent create that loses the unique-index race is retried as an
// update, so two scheduler passes cannot fail each other.
func (r *ChannelHealthRepository) upsert(
	ctx context.Context,
	channel, target string,
	update func(*ent.ChannelHealthUpdateOne),
	create func(*ent.ChannelHealthCreate),
) error {
	row, err := r.client.ChannelHealth.Query().
		Where(
			channelhealth.ChannelEQ(channel),
			channelhealth.TargetEQ(target),
		).
		Only(ctx)
	switch {
	case err == nil:
		b := r.client.ChannelHealth.UpdateOneID(row.ID)
		update(b)
		return b.Exec(ctx)
	case !ent.IsNotFound(err):
		return err
	}
	c := r.client.ChannelHealth.Create().SetChannel(channel).SetTarget(target)
	create(c)
	if _, err := c.Save(ctx); err != nil {
		if !ent.IsConstraintError(err) {
			return err
		}
		// Lost the race: the row now exists, so record the result on it.
		row, qErr := r.client.ChannelHealth.Query().
			Where(
				channelhealth.ChannelEQ(channel),
				channelhealth.TargetEQ(target),
			).
			Only(ctx)
		if qErr != nil {
			return qErr
		}
		b := r.client.ChannelHealth.UpdateOneID(row.ID)
		update(b)
		return b.Exec(ctx)
	}
	return nil
}

// ListAll returns every health row in one query. The web layer indexes the
// result and never queries per channel.
func (r *ChannelHealthRepository) ListAll(ctx context.Context) ([]*ent.ChannelHealth, error) {
	rows, err := r.client.ChannelHealth.Query().All(ctx)
	if err != nil {
		slog.Error("channel health: list all", "source", "repository", "error", err)
		return nil, err
	}
	return rows, nil
}
