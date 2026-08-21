package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// GroupNote is a dated free-form note attached to a group, displayed in a
// timeline on the group detail page.
type GroupNote struct {
	ent.Schema
}

func (GroupNote) Fields() []ent.Field {
	return []ent.Field{
		field.Text("note").NotEmpty(),
		field.Time("note_date"),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (GroupNote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("group", Group.Type).Ref("notes").Unique().Required(),
	}
}
