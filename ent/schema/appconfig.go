package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// AppConfig stores all application configuration in a single singleton row
// (id=1). Every field is optional+nullable so that a NULL column means
// "fall back to the environment variable value", while a non-NULL column
// means "the database value wins". This lets administrators override any
// environment-defined setting from the admin UI without editing env files.
//
// Boot-critical fields (port, data_dir, backup_dir, backup_retention_days)
// are persisted here for display and next-restart application, but the
// running process keeps the value it booted with (see config.OverlayDB).
type AppConfig struct {
	ent.Schema
}

func (AppConfig) Fields() []ent.Field {
	return []ent.Field{
		field.Int("port").Optional().Nillable(),
		field.String("data_dir").Optional().Nillable(),
		field.Int("scheduler_hour").Optional().Nillable(),
		field.Int("reminder_days").Optional().Nillable(),
		field.String("log_level").Optional().Nillable(),
		field.Int("log_buffer_size").Optional().Nillable(),
		field.String("otel_endpoint").Optional().Nillable(),

		field.String("backup_dir").Optional().Nillable(),
		field.Int("backup_retention_days").Optional().Nillable(),

		field.String("smtp_host").Optional().Nillable(),
		field.Int("smtp_port").Optional().Nillable(),
		field.String("smtp_user").Optional().Nillable(),
		field.String("smtp_pass").Optional().Nillable(),
		field.Bool("smtp_tls").Optional().Nillable(),
		field.Int("smtp_timeout").Optional().Nillable(),
		field.String("notify_email").Optional().Nillable(),

		field.String("gotify_url").Optional().Nillable(),
		field.String("gotify_token").Optional().Nillable(),

		field.String("telegram_bot_token").Optional().Nillable(),
		field.String("telegram_chat_id").Optional().Nillable(),

		field.String("umami_url").Optional().Nillable(),
		field.String("umami_website_id").Optional().Nillable(),

		field.Bool("eink_mode").Optional().Nillable(),

		field.Time("updated_at").Optional().Nillable(),
	}
}

func (AppConfig) Edges() []ent.Edge {
	return nil
}