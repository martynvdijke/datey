package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Subscription is a remote iCalendar (ICS) feed that Datey polls and whose
// events it keeps in sync. Events created from a subscription carry its
// subscription_id; removing the subscription detaches them instead of
// deleting them.
type Subscription struct {
	ent.Schema
}

func (Subscription) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("url").NotEmpty(),
		field.Bool("enabled").Default(true),
		field.Time("last_attempt_at").Optional().Nillable(),
		field.Time("last_success_at").Optional().Nillable(),
		field.Text("last_error").Optional().Default(""),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Subscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("events", Event.Type),
	}
}

func (Subscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
	}
}
