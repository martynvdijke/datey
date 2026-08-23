package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GiftIdea struct {
	ent.Schema
}

func (GiftIdea) Fields() []ent.Field {
	return []ent.Field{
		field.String("title").NotEmpty(),
		field.Text("notes").Optional().Default(""),
		field.Int("price_cents").Optional().Nillable(),
		field.String("url").Optional().Default(""),
		field.Enum("status").Values("idea", "purchased", "given", "archived").Default("idea"),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (GiftIdea) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).Ref("giftIdeas").Unique().Required(),
	}
}

func (GiftIdea) Indexes() []ent.Index {
	return []ent.Index{
		index.Edges("person").Fields("status"),
	}
}
