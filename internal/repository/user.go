package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/user"
)

type UserRepository struct {
	client *ent.Client
}

func NewUserRepository(client *ent.Client) *UserRepository {
	return &UserRepository{client: client}
}

func (r *UserRepository) Create(ctx context.Context, username, passwordHash string, role user.Role) (*ent.User, error) {
	return r.client.User.Create().
		SetUsername(username).
		SetPasswordHash(passwordHash).
		SetRole(role).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *UserRepository) GetByID(ctx context.Context, id int) (*ent.User, error) {
	return r.client.User.Get(ctx, id)
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*ent.User, error) {
	return r.client.User.Query().
		Where(user.UsernameEQ(username)).
		Only(ctx)
}

func (r *UserRepository) List(ctx context.Context) ([]*ent.User, error) {
	return r.client.User.Query().
		Order(ent.Asc(user.FieldUsername)).
		All(ctx)
}

func (r *UserRepository) Delete(ctx context.Context, id int) error {
	return r.client.User.DeleteOneID(id).Exec(ctx)
}

func (r *UserRepository) Exists(ctx context.Context) (bool, error) {
	return r.client.User.Query().Exist(ctx)
}

func (r *UserRepository) GetEinkMode(ctx context.Context, id int) (bool, error) {
	u, err := r.client.User.Get(ctx, id)
	if err != nil {
		return false, err
	}
	return u.EinkMode, nil
}

func (r *UserRepository) SetEinkMode(ctx context.Context, id int, enabled bool) error {
	return r.client.User.UpdateOneID(id).SetEinkMode(enabled).Exec(ctx)
}

func (r *UserRepository) UpdateEinkMode(ctx context.Context, id int) (bool, error) {
	current, err := r.GetEinkMode(ctx, id)
	if err != nil {
		return false, err
	}
	newVal := !current
	if err := r.SetEinkMode(ctx, id, newVal); err != nil {
		return false, err
	}
	return newVal, nil
}
