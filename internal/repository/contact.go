// Deprecated: ContactRepository is kept for backward compatibility during the
// transition to Person. New code should use PersonRepository (person.go) instead.
// The Contact schema remains active for existing data; the MigrateContactsToPeople
// function in db/db.go handles copying existing contacts to the new people table.

package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/contact"
)

type ContactRepository struct {
	client *ent.Client
}

func NewContactRepository(client *ent.Client) *ContactRepository {
	return &ContactRepository{client: client}
}

func (r *ContactRepository) Create(ctx context.Context, name, notes string) (*ent.Contact, error) {
	return r.client.Contact.Create().
		SetName(name).
		SetNotes(notes).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *ContactRepository) Get(ctx context.Context, id int) (*ent.Contact, error) {
	return r.client.Contact.Get(ctx, id)
}

func (r *ContactRepository) List(ctx context.Context) ([]*ent.Contact, error) {
	return r.client.Contact.Query().
		Order(ent.Asc(contact.FieldName)).
		All(ctx)
}

func (r *ContactRepository) Search(ctx context.Context, q string) ([]*ent.Contact, error) {
	return r.client.Contact.Query().
		Where(contact.NameContainsFold(q)).
		Order(ent.Asc(contact.FieldName)).
		All(ctx)
}

func (r *ContactRepository) Update(ctx context.Context, id int, name, notes string) (*ent.Contact, error) {
	return r.client.Contact.UpdateOneID(id).
		SetName(name).
		SetNotes(notes).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *ContactRepository) FindByName(ctx context.Context, name string) (*ent.Contact, error) {
	return r.client.Contact.Query().
		Where(contact.Name(name)).
		Only(ctx)
}

func (r *ContactRepository) Delete(ctx context.Context, id int) error {
	return r.client.Contact.DeleteOneID(id).Exec(ctx)
}
