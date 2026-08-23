package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Tag struct {
	ent.Schema
}

func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().Unique(),
		field.Time("created_at").Default(time.Now),
	}
}

func (Tag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("people", Person.Type).Ref("tags"),
	}
}
