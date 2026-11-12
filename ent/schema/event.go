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
		field.String("frequency").Default("annual"),
		field.Int("interval").Default(1).Min(1).Max(120),
		field.Time("end_date").Optional().Nillable(),
		field.Int("occurrence_count").Optional().Nillable().Min(1),
		field.Int("subscription_id").Optional().Nillable(),
		field.String("external_uid").Optional().Default(""),
		// rule_kind makes the event date computable from a calendar rule
		// (nth_weekday, last_weekday, fixed, or an Easter-based pattern) instead
		// of the stored month/day. Empty means a plain dated event.
		field.String("rule_kind").Optional().Default(""),
		field.Int("rule_ordinal").Optional().Nillable().Min(1).Max(5),
		field.Int("rule_weekday").Optional().Nillable().Min(0).Max(6),
	}
}

func (Event) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("contact", Contact.Type).Ref("events").Unique(),
		edge.From("person", Person.Type).Ref("events").Unique(),
		edge.From("group", Group.Type).Ref("events").Unique(),
		edge.To("notification_logs", NotificationLog.Type),
		edge.From("subscription", Subscription.Type).Ref("events").Unique().Field("subscription_id"),
	}
}

func (Event) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("date"),
		index.Fields("type"),
	}
}
