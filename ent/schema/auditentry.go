package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AuditEntry struct {
	ent.Schema
}

func (AuditEntry) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at"),
		field.String("actor_username").NotEmpty(),
		field.String("action").NotEmpty(),
		field.String("target").NotEmpty().Default(""),
		field.String("source_ip").Optional().Default(""),
	}
}

func (AuditEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("action"),
	}
}

func (AuditEntry) Edges() []ent.Edge {
	return nil
}
