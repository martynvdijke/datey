package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserNotificationChannel struct {
	ent.Schema
}

func (UserNotificationChannel) Fields() []ent.Field {
	return []ent.Field{
		field.String("channel_type").NotEmpty(),
		field.Text("target").Optional().Default(""),
		field.Bool("enabled").Default(true),
	}
}

func (UserNotificationChannel) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("notification_channels").Unique().Required(),
	}
}

func (UserNotificationChannel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel_type").Edges("user").Unique(),
	}
}
