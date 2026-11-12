package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Snooze suppresses reminders for one event occurrence until a later time.
// Occasion is keyed by the occurrence date so multi-occurrence schedules
// snooze independently.
type Snooze struct {
	ent.Schema
}

func (Snooze) Fields() []ent.Field {
	return []ent.Field{
		field.Int("event_id"),
		field.Time("occasion"),
		field.Time("snoozed_until"),
		field.Int("user_id").Default(0),
		field.Time("created_at"),
	}
}

func (Snooze) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("event_id", "occasion", "user_id").Unique(),
		index.Fields("snoozed_until"),
	}
}
