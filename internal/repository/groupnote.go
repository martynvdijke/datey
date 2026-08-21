package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/group"
	"github.com/datey/datey/ent/groupnote"
)

// GroupNoteRepository stores dated free-form notes attached to groups.
type GroupNoteRepository struct {
	client *ent.Client
}

func NewGroupNoteRepository(client *ent.Client) *GroupNoteRepository {
	return &GroupNoteRepository{client: client}
}

func (r *GroupNoteRepository) Create(ctx context.Context, groupID int, note string, noteDate time.Time) (*ent.GroupNote, error) {
	return r.client.GroupNote.Create().
		SetGroupID(groupID).
		SetNote(note).
		SetNoteDate(noteDate).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *GroupNoteRepository) Get(ctx context.Context, id int) (*ent.GroupNote, error) {
	return r.client.GroupNote.Get(ctx, id)
}

func (r *GroupNoteRepository) Update(ctx context.Context, id int, note string, noteDate time.Time) (*ent.GroupNote, error) {
	return r.client.GroupNote.UpdateOneID(id).
		SetNote(note).
		SetNoteDate(noteDate).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *GroupNoteRepository) Delete(ctx context.Context, id int) error {
	return r.client.GroupNote.DeleteOneID(id).Exec(ctx)
}

// ListByGroup returns the group's notes ordered oldest first (timeline order).
func (r *GroupNoteRepository) ListByGroup(ctx context.Context, groupID int) ([]*ent.GroupNote, error) {
	return r.client.GroupNote.Query().
		Where(groupnote.HasGroupWith(group.IDEQ(groupID))).
		Order(ent.Asc(groupnote.FieldNoteDate), ent.Asc(groupnote.FieldCreatedAt)).
		All(ctx)
}
