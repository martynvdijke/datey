package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type NotificationDelivery struct {
	ent.Schema
}

func (NotificationDelivery) Fields() []ent.Field {
	return []ent.Field{
		field.String("channel").NotEmpty(),
		field.String("status").Default("pending"),
		field.Time("sent_at").Optional().Nillable(),
		field.Text("error_message").Optional().Default(""),
	}
}

func (NotificationDelivery) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("notification", OneTimeNotification.Type).Ref("deliveries").Unique().Required(),
	}
}
