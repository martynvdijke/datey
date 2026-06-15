package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Group struct {
	ent.Schema
}

func (Group) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().Unique(),
		field.Text("description").Optional().Default(""),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (Group) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("people", Person.Type).Ref("groups"),
	}
}
