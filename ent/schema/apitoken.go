package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ApiToken is a bearer API token used to authenticate non-browser clients
// (scripts, integrations) against protected mutation routes.
//
// Only the SHA-256 hash of the token secret is stored; the raw secret is
// shown to the owner exactly once at creation/rotation time.
type ApiToken struct {
	ent.Schema
}

func (ApiToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("token_hash").Unique().NotEmpty(),
		field.String("name").NotEmpty().Default("api token"),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("created_at"),
	}
}

func (ApiToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("api_tokens").Unique().Required(),
	}
}

func (ApiToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token_hash"),
		index.Edges("user"),
	}
}
