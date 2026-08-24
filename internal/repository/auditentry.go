package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/auditentry"
)

type AuditEntryRepository struct {
	client *ent.Client
}

func NewAuditEntryRepository(client *ent.Client) *AuditEntryRepository {
	return &AuditEntryRepository{client: client}
}

func (r *AuditEntryRepository) Append(ctx context.Context, e *ent.AuditEntry) (*ent.AuditEntry, error) {
	return r.client.AuditEntry.Create().
		SetCreatedAt(e.CreatedAt).
		SetActorUsername(e.ActorUsername).
		SetAction(e.Action).
		SetTarget(e.Target).
		SetSourceIP(e.SourceIP).
		Save(ctx)
}

type AuditFilter struct {
	Actor  string
	Action string
	From   *time.Time
	To     *time.Time
	Limit  int
	Offset int
}

func (r *AuditEntryRepository) List(ctx context.Context, f AuditFilter) ([]*ent.AuditEntry, error) {
	q := r.client.AuditEntry.Query()
	if f.Actor != "" {
		q = q.Where(auditentry.ActorUsernameEQ(f.Actor))
	}
	if f.Action != "" {
		q = q.Where(auditentry.ActionEQ(f.Action))
	}
	if f.From != nil {
		q = q.Where(auditentry.CreatedAtGTE(*f.From))
	}
	if f.To != nil {
		q = q.Where(auditentry.CreatedAtLTE(*f.To))
	}
	q = q.Order(ent.Desc(auditentry.FieldCreatedAt))
	if f.Limit > 0 {
		q = q.Limit(f.Limit).Offset(f.Offset)
	} else if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	return q.All(ctx)
}

func (r *AuditEntryRepository) Count(ctx context.Context, f AuditFilter) (int, error) {
	q := r.client.AuditEntry.Query()
	if f.Actor != "" {
		q = q.Where(auditentry.ActorUsernameEQ(f.Actor))
	}
	if f.Action != "" {
		q = q.Where(auditentry.ActionEQ(f.Action))
	}
	if f.From != nil {
		q = q.Where(auditentry.CreatedAtGTE(*f.From))
	}
	if f.To != nil {
		q = q.Where(auditentry.CreatedAtLTE(*f.To))
	}
	return q.Count(ctx)
}

func (r *AuditEntryRepository) PruneToCap(ctx context.Context, cap int) (int, error) {
	if cap < 1 {
		return 0, nil
	}
	total, err := r.client.AuditEntry.Query().Count(ctx)
	if err != nil {
		return 0, err
	}
	if total <= cap {
		return 0, nil
	}
	toDelete := total - cap
	olds, err := r.client.AuditEntry.Query().Order(ent.Asc(auditentry.FieldCreatedAt)).Limit(toDelete).All(ctx)
	if err != nil {
		return 0, err
	}
	for _, e := range olds {
		if err := r.client.AuditEntry.DeleteOneID(e.ID).Exec(ctx); err != nil {
			return 0, err
		}
	}
	return toDelete, nil
}
