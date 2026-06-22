package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// MigrationLog records one-time data migrations that have been applied,
// so they are not re-run on every startup.
type MigrationLog struct {
	ent.Schema
}

func (MigrationLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique().NotEmpty(),
		field.Time("applied_at"),
	}
}

func (MigrationLog) Edges() []ent.Edge {
	return nil
}
