package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Relationship tracks typed connections between people.
type Relationship struct {
	ent.Schema
}

func (Relationship) Fields() []ent.Field {
	return []ent.Field{
		field.Int("from_id"),
		field.Int("to_id"),
		field.Enum("type").Values("partner", "parent", "sibling", "custom"),
		field.String("label").Optional().Nillable().MaxLen(50),
	}
}

func (Relationship) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("from_person", Person.Type).Ref("outgoing_relationships").Field("from_id").Required().Unique(),
		edge.From("to_person", Person.Type).Ref("incoming_relationships").Field("to_id").Required().Unique(),
	}
}

func (Relationship) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("from_id"),
		index.Fields("to_id"),
		index.Fields("from_id", "to_id", "type").Unique(),
	}
}
