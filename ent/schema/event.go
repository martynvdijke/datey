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
		field.Text("notes").Optional().Default(""),
		field.Ints("reminder_days").Optional(),
		field.Time("created_at"),
		field.String("calendar_system").Default("gregorian"),
		field.Int("lunar_month").Optional().Nillable().Min(1).Max(12),
		field.Int("lunar_day").Optional().Nillable().Min(1).Max(30),
		field.Bool("lunar_leap").Default(false),
	}
}

func (Event) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("contact", Contact.Type).Ref("events").Unique(),
		edge.From("person", Person.Type).Ref("events").Unique(),
		edge.From("group", Group.Type).Ref("events").Unique(),
		edge.To("notification_logs", NotificationLog.Type),
	}
}

func (Event) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("date"),
		index.Fields("type"),
	}
}
