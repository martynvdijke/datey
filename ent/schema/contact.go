package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Contact struct {
	ent.Schema
}

func (Contact) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.Text("notes").Optional().Default(""),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (Contact) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("events", Event.Type),
	}
}
