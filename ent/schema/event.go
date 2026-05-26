package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Event struct {
	ent.Schema
}

func (Event) Fields() []ent.Field {
	return []ent.Field{
		field.String("type").NotEmpty(),
		field.Time("date"),
		field.Text("description").Optional().Default(""),
		field.Time("created_at"),
	}
}

func (Event) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("contact", Contact.Type).Ref("events").Unique().Required(),
		edge.To("notification_logs", NotificationLog.Type),
	}
}

func (Event) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("date"),
		index.Fields("type"),
	}
}
