package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/group"
	"github.com/datey/datey/ent/person"
)

type GroupRepository struct {
	client *ent.Client
}

func NewGroupRepository(client *ent.Client) *GroupRepository {
	return &GroupRepository{client: client}
}

func (r *GroupRepository) Create(ctx context.Context, name, description string) (*ent.Group, error) {
	return r.client.Group.Create().
		SetName(name).
		SetDescription(description).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *GroupRepository) GetByID(ctx context.Context, id int) (*ent.Group, error) {
	return r.client.Group.Get(ctx, id)
}

func (r *GroupRepository) List(ctx context.Context) ([]*ent.Group, error) {
	return r.client.Group.Query().
		Order(ent.Asc(group.FieldName)).
		All(ctx)
}

func (r *GroupRepository) Delete(ctx context.Context, id int) error {
	// Remove all people from the group first
	if err := r.client.Group.UpdateOneID(id).ClearPeople().Exec(ctx); err != nil {
		return err
	}
	return r.client.Group.DeleteOneID(id).Exec(ctx)
}

func (r *GroupRepository) AddPerson(ctx context.Context, groupID, personID int) error {
	return r.client.Group.UpdateOneID(groupID).
		AddPersonIDs(personID).
		Exec(ctx)
}

func (r *GroupRepository) RemovePerson(ctx context.Context, groupID, personID int) error {
	return r.client.Group.UpdateOneID(groupID).
		RemovePersonIDs(personID).
		Exec(ctx)
}

func (r *GroupRepository) ListByPerson(ctx context.Context, personID int) ([]*ent.Group, error) {
	return r.client.Person.Query().
		Where(person.IDEQ(personID)).
		QueryGroups().
		All(ctx)
}

func (r *GroupRepository) ListPeopleInGroup(ctx context.Context, groupID int) ([]*ent.Person, error) {
	return r.client.Group.Query().
		Where(group.IDEQ(groupID)).
		QueryPeople().
		All(ctx)
}
