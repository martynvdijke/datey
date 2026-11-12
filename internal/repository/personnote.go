package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/person"
	"github.com/datey/datey/ent/personnote"
)

// PersonNoteRepository stores dated free-form notes attached to people.
type PersonNoteRepository struct {
	client *ent.Client
}

func NewPersonNoteRepository(client *ent.Client) *PersonNoteRepository {
	return &PersonNoteRepository{client: client}
}

func (r *PersonNoteRepository) Create(ctx context.Context, personID int, note string, noteDate time.Time) (*ent.PersonNote, error) {
	return r.client.PersonNote.Create().
		SetPersonID(personID).
		SetNote(note).
		SetNoteDate(noteDate).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

// Get returns a note only when it belongs to the given person, preventing
// cross-person access.
func (r *PersonNoteRepository) Get(ctx context.Context, personID, noteID int) (*ent.PersonNote, error) {
	return r.client.PersonNote.Query().
		Where(
			personnote.IDEQ(noteID),
			personnote.HasPersonWith(person.IDEQ(personID)),
		).
		Only(ctx)
}

// Update modifies a note only when it belongs to the given person.
func (r *PersonNoteRepository) Update(ctx context.Context, personID, noteID int, note string, noteDate time.Time) (*ent.PersonNote, error) {
	return r.client.PersonNote.UpdateOneID(noteID).
		Where(personnote.HasPersonWith(person.IDEQ(personID))).
		SetNote(note).
		SetNoteDate(noteDate).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

// Delete removes a note only when it belongs to the given person.
func (r *PersonNoteRepository) Delete(ctx context.Context, personID, noteID int) error {
	n, err := r.client.PersonNote.Delete().
		Where(
			personnote.IDEQ(noteID),
			personnote.HasPersonWith(person.IDEQ(personID)),
		).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("person note not found")
	}
	return nil
}

// ListAll returns every person note with its owner eager-loaded, ordered by
// note date then ID. Used by whole-dataset exports.
func (r *PersonNoteRepository) ListAll(ctx context.Context) ([]*ent.PersonNote, error) {
	return r.client.PersonNote.Query().
		WithPerson().
		Order(ent.Asc(personnote.FieldNoteDate), ent.Asc(personnote.FieldID)).
		All(ctx)
}

// ListByPerson returns the person's notes ordered oldest first (timeline order).
func (r *PersonNoteRepository) ListByPerson(ctx context.Context, personID int) ([]*ent.PersonNote, error) {
	return r.client.PersonNote.Query().
		Where(personnote.HasPersonWith(person.IDEQ(personID))).
		Order(ent.Asc(personnote.FieldNoteDate), ent.Asc(personnote.FieldCreatedAt)).
		All(ctx)
}
