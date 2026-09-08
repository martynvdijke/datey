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

// GetByEmail finds a user by verified OIDC email.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*ent.User, error) {
	return r.client.User.Query().
		Where(user.EmailEQ(email)).
		Only(ctx)
}

// GetByOIDCSub finds a user by provider subject.
func (r *UserRepository) GetByOIDCSub(ctx context.Context, sub string) (*ent.User, error) {
	return r.client.User.Query().
		Where(user.OidcSubEQ(sub)).
		Only(ctx)
}

// CreateOIDC provisions a user from OIDC claims. passwordHash must be an
// unusable random value — OIDC users never log in with a password.
func (r *UserRepository) CreateOIDC(ctx context.Context, username, email, oidcSub, passwordHash string, role user.Role) (*ent.User, error) {
	q := r.client.User.Create().
		SetUsername(username).
		SetPasswordHash(passwordHash).
		SetRole(role).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now())
	if email != "" {
		q.SetEmail(email)
	}
	if oidcSub != "" {
		q.SetOidcSub(oidcSub)
	}
	return q.Save(ctx)
}

// LinkOIDCSub attaches a provider subject + email to an existing user.
func (r *UserRepository) LinkOIDCSub(ctx context.Context, id int, email, oidcSub string) error {
	q := r.client.User.UpdateOneID(id).SetUpdatedAt(time.Now())
	if email != "" {
		q.SetEmail(email)
	}
	if oidcSub != "" {
		q.SetOidcSub(oidcSub)
	}
	return q.Exec(ctx)
}

// SetRole updates the admin flag (used for groups->admin sync).
func (r *UserRepository) SetRole(ctx context.Context, id int, role user.Role) error {
	return r.client.User.UpdateOneID(id).
		SetRole(role).
		SetUpdatedAt(time.Now()).
		Exec(ctx)
}

func (r *UserRepository) List(ctx context.Context) ([]*ent.User, error) {
	return r.client.User.Query().
		Order(ent.Asc(user.FieldUsername)).
		All(ctx)
}

func (r *UserRepository) Delete(ctx context.Context, id int) error {
	return r.client.User.DeleteOneID(id).Exec(ctx)
}

// UpdatePassword replaces the stored bcrypt hash for the given user.
func (r *UserRepository) UpdatePassword(ctx context.Context, id int, passwordHash string) error {
	return r.client.User.UpdateOneID(id).
		SetPasswordHash(passwordHash).
		SetUpdatedAt(time.Now()).
		Exec(ctx)
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

func (r *UserRepository) SetLocale(ctx context.Context, id int, locale string) error {
	if locale == "" {
		return r.client.User.UpdateOneID(id).ClearLocale().Exec(ctx)
	}
	return r.client.User.UpdateOneID(id).SetLocale(locale).Exec(ctx)
}

func (r *UserRepository) GetLocale(ctx context.Context, id int) (string, error) {
	u, err := r.client.User.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if u.Locale == nil {
		return "", nil
	}
	return *u.Locale, nil
}
