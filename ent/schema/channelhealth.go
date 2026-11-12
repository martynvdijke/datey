package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChannelHealth records the most recent delivery outcome for one notification
// channel + target pair so the settings UI can surface failures instead of
// silently dropping reminders.
type ChannelHealth struct {
	ent.Schema
}

func (ChannelHealth) Fields() []ent.Field {
	return []ent.Field{
		field.String("channel").NotEmpty(),
		field.String("target").Optional().Default(""),
		field.Time("last_success_at").Optional().Nillable(),
		field.Time("last_failure_at").Optional().Nillable(),
		field.Text("last_error").Optional().Default(""),
		field.Int("consecutive_failures").Default(0),
	}
}

func (ChannelHealth) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel", "target").Unique(),
	}
}
