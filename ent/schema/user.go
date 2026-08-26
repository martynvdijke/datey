package schema

import (
	"regexp"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("username").Unique().NotEmpty(),
		field.String("password_hash").NotEmpty(),
		field.Enum("role").Values("admin", "user").Default("user"),
		field.Bool("eink_mode").Default(false),
		field.String("locale").Optional().Nillable().Match(regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)),
		field.Enum("notification_scope_mode").Values("all", "selected").Default("all"),
		field.Text("notification_scope_group_ids").Optional().Default(""),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("sessions", Session.Type),
		edge.To("push_subscriptions", PushSubscription.Type),
		edge.To("password_reset_tokens", PasswordResetToken.Type),
		edge.To("notification_channels", UserNotificationChannel.Type),
		edge.To("api_tokens", ApiToken.Type),
	}
}
