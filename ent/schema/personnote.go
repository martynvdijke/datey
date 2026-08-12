package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// PersonNote is a dated free-form note attached to a person, displayed in a
// timeline on the person detail page.
type PersonNote struct {
	ent.Schema
}

func (PersonNote) Fields() []ent.Field {
	return []ent.Field{
		field.Text("note").NotEmpty(),
		field.Time("note_date"),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (PersonNote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).Ref("timeline").Unique().Required(),
	}
}
