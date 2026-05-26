package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type RecurringRule struct {
	ent.Schema
}

func (RecurringRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("pattern_type").NotEmpty(),
		field.Int("nth").Optional().Default(0),
		field.Int("weekday").Optional().Default(0),
		field.Int("month").Optional().Default(0),
		field.Int("day").Optional().Default(0),
		field.Bool("enabled").Default(true),
		field.Time("created_at"),
	}
}
