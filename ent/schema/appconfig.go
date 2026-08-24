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
		field.Time("last_scheduler_run").Optional().Nillable(),
		field.Bool("scheduler_catchup").Optional().Nillable(),
		field.String("date_variant").Optional().Nillable(),
		field.Bool("reminder_digest").Optional().Nillable(),
		field.String("reminder_stages").Optional().Nillable(),
		field.String("timezone").Optional().Nillable(),
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

		field.String("ntfy_url").Optional().Nillable(),
		field.String("ntfy_topic").Optional().Nillable(),
		field.String("ntfy_token").Optional().Nillable(),
		field.Int("ntfy_priority").Optional().Nillable(),

		field.String("webhook_url").Optional().Nillable(),
		field.String("webhook_secret").Optional().Nillable(),

		field.String("discord_webhook_url").Optional().Nillable(),
		field.String("slack_webhook_url").Optional().Nillable(),
		field.String("matrix_homeserver_url").Optional().Nillable(),
		field.String("matrix_access_token").Optional().Nillable(),
		field.String("matrix_room_id").Optional().Nillable(),

		field.String("umami_url").Optional().Nillable(),
		field.String("umami_website_id").Optional().Nillable(),

		field.Bool("eink_mode").Optional().Nillable(),

		field.Bool("ical_enabled").Optional().Nillable(),
		field.String("ical_event_start").Optional().Nillable(),
		field.Int("ical_duration_minutes").Optional().Nillable(),
		field.String("ical_feed_key").Optional().Nillable(),

		field.Bool("rss_enabled").Optional().Nillable(),
		field.String("rss_feed_key").Optional().Nillable(),

		field.Bool("upcoming_api_enabled").Optional().Nillable(),
		field.String("upcoming_api_key").Optional().Nillable(),

		field.Bool("homeassistant_enabled").Optional().Nillable(),
		field.String("homeassistant_key").Optional().Nillable(),

		field.Bool("push_enabled").Optional().Nillable(),
		field.String("push_vapid_public_key").Optional().Nillable(),
		field.String("push_vapid_private_key").Optional().Nillable(),

		field.String("immich_url").Optional().Nillable(),
		field.String("immich_api_key").Optional().Nillable(),

		field.Bool("carddav_enabled").Optional().Nillable(),
		field.String("carddav_url").Optional().Nillable(),
		field.String("carddav_username").Optional().Nillable(),
		field.String("carddav_password").Optional().Nillable(),
		field.String("carddav_sync_token").Optional().Nillable(),
		field.Time("carddav_last_sync").Optional().Nillable(),
		field.String("carddav_delete_policy").Optional().Nillable(),

		field.Bool("google_contacts_enabled").Optional().Nillable(),
		field.String("google_client_id").Optional().Nillable(),
		field.String("google_client_secret").Optional().Nillable(),
		field.String("google_refresh_token").Optional().Nillable(),
		field.String("google_sync_token").Optional().Nillable(),
		field.Time("google_last_sync").Optional().Nillable(),
		field.String("google_delete_policy").Optional().Nillable(),
		field.String("google_oauth_state").Optional().Nillable(),

		field.Time("updated_at").Optional().Nillable(),

		field.Int("audit_retention_max").Optional().Nillable(),
	}
}

func (AppConfig) Edges() []ent.Edge {
	return nil
}
