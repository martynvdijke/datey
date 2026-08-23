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
		field.String("photo_path").Optional().Nillable(),
		field.String("photo_content_type").Optional().Nillable(),
		field.Time("photo_updated_at").Optional().Nillable(),
		field.String("photo_source").Optional().Nillable(),
		field.String("carddav_uid").Optional().Nillable(),
		field.String("carddav_href").Optional().Nillable(),
		field.String("carddav_etag").Optional().Nillable(),
		field.String("carddav_rev").Optional().Nillable(),
		field.Time("carddav_last_modified").Optional().Nillable(),
		field.Bool("carddav_pending_sync").Default(false),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (Person) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("events", Event.Type),
		edge.To("groups", Group.Type),
		edge.To("timeline", PersonNote.Type),
		edge.To("tags", Tag.Type),
		edge.To("giftIdeas", GiftIdea.Type),
	}
}
