package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// PasswordResetToken holds a one-time token used to reset a user's password.
// Only a SHA-256 hash of the token is stored; the raw token is emailed once
// and can never be recovered from the database.
type PasswordResetToken struct {
	ent.Schema
}

func (PasswordResetToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("token_hash").Unique().NotEmpty(),
		field.Time("expires_at"),
		field.Bool("used").Default(false),
		field.Time("created_at"),
	}
}

func (PasswordResetToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("password_reset_tokens").Unique().Required(),
	}
}
