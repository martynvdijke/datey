package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Person struct {
	ent.Schema
}

func (Person) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().Unique(),
		field.Text("notes").Optional().Default(""),
		field.Text("vcard_data").Optional(),
		field.Bool("notify_birthdays").Default(true),
		field.String("timezone").Optional().Default(""),
		field.Ints("reminder_days").Optional(),
		field.String("immich_person_id").Optional().Nillable(),
		field.Bool("immich_photo_disabled").Default(false),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (Person) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("events", Event.Type),
		edge.To("groups", Group.Type),
		edge.To("timeline", PersonNote.Type),
	}
}
