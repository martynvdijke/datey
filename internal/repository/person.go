package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/person"
)

type PersonRepository struct {
	client *ent.Client
}

func NewPersonRepository(client *ent.Client) *PersonRepository {
	return &PersonRepository{client: client}
}

func (r *PersonRepository) Create(ctx context.Context, name, notes string) (*ent.Person, error) {
	return r.client.Person.Create().
		SetName(name).
		SetNotes(notes).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *PersonRepository) Get(ctx context.Context, id int) (*ent.Person, error) {
	return r.client.Person.Get(ctx, id)
}

func (r *PersonRepository) List(ctx context.Context) ([]*ent.Person, error) {
	return r.client.Person.Query().
		Order(ent.Asc(person.FieldName)).
		All(ctx)
}

func (r *PersonRepository) Search(ctx context.Context, q string) ([]*ent.Person, error) {
	return r.client.Person.Query().
		Where(person.NameContainsFold(q)).
		Order(ent.Asc(person.FieldName)).
		All(ctx)
}

func (r *PersonRepository) Update(ctx context.Context, id int, name, notes string) (*ent.Person, error) {
	return r.client.Person.UpdateOneID(id).
		SetName(name).
		SetNotes(notes).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *PersonRepository) FindByName(ctx context.Context, name string) (*ent.Person, error) {
	return r.client.Person.Query().
		Where(person.Name(name)).
		Only(ctx)
}

func (r *PersonRepository) Delete(ctx context.Context, id int) error {
	return r.client.Person.DeleteOneID(id).Exec(ctx)
}
