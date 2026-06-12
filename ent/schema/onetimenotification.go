package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type OneTimeNotification struct {
	ent.Schema
}

func (OneTimeNotification) Fields() []ent.Field {
	return []ent.Field{
		field.Text("message").NotEmpty(),
		field.Time("scheduled_at"),
		field.String("status").Default("pending"),
		field.Time("created_at"),
		field.Time("sent_at").Optional().Nillable(),
		field.String("channel_targets").Optional().Default(""),
	}
}

func (OneTimeNotification) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("deliveries", NotificationDelivery.Type),
	}
}
