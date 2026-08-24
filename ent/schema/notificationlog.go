package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type NotificationLog struct {
	ent.Schema
}

func (NotificationLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("channel").NotEmpty(),
		field.Time("sent_at"),
		field.String("date_key").NotEmpty(),
		field.Int("user_id").Optional().Default(0),
	}
}

func (NotificationLog) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("event", Event.Type).Ref("notification_logs").Unique().Required(),
	}
}

func (NotificationLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel", "date_key"),
		index.Fields("channel", "date_key", "user_id"),
	}
}
